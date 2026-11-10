package capability_registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
)

const PluginServiceAuthType = "core_service_ticket"
const PluginServiceIdentityHeader = "X-PowerX-Service-Invocation"

// UsesConstrainedServicePayload 供传输层识别固定服务合同，避免往业务 DTO 注入代理头。
func (s *RegistryService) UsesConstrainedServicePayload(ctx context.Context, id, preferred string) (bool, error) {
	view, err := s.GetCapability(ctx, id, false)
	if err != nil {
		return false, err
	}
	if hasServiceTicketBinding(view.Record) {
		return true, nil
	}
	if strings.TrimSpace(preferred) == "" {
		var policy struct {
			Prefer string `json:"prefer"`
		}
		if json.Unmarshal(view.Record.Policy, &policy) == nil && policy.Prefer == "core_internal" {
			return true, nil
		}
	}
	return false, nil
}

func hasServiceTicketBinding(record *models.CapabilityRecord) bool {
	var bindings []models.ProtocolBinding
	if record == nil || json.Unmarshal(record.Protocols, &bindings) != nil {
		return false
	}
	for _, p := range bindings {
		if p.AuthType == PluginServiceAuthType {
			return true
		}
	}
	return false
}

// ServiceInvocationBinding 仅由 Core 从正式能力绑定和实际请求构造。
type ServiceInvocationBinding struct {
	TenantUUID        string `json:"tenant_uuid"`
	TargetPluginID    string `json:"target_plugin_id"`
	CapabilityID      string `json:"capability_id"`
	CapabilityVersion string `json:"capability_version"`
	ProviderVersion   string `json:"provider_version"`
	CapabilitiesHash  string `json:"capabilities_hash"`
	ProtocolHash      string `json:"protocol_hash"`
	Method            string `json:"method"`
	Path              string `json:"path"`
	BodySHA256        string `json:"body_sha256"`
	RequestID         string `json:"request_id"`
	TraceID           string `json:"trace_id"`
}

type ServiceInvocationIdentityIssuer interface {
	IssueServiceInvocation(context.Context, ServiceInvocationBinding) (string, error)
	DiscardServiceInvocation(context.Context, string)
}

// Resolver 使用进程内插件挂载表解析运行地址；业务 payload 无权指定地址。
type PluginServiceTargetResolver func(ctx context.Context, tenant, pluginID, providerVersion, requestPath string) (string, error)

func (s *InvocationService) BindPluginServiceTargetResolver(resolver PluginServiceTargetResolver) {
	s.pluginTargetMu.Lock()
	defer s.pluginTargetMu.Unlock()
	s.pluginTargetResolver = resolver
}

func pluginServiceError(status int, code string) error {
	return dto.NewErrorWithCode(status, code, code, nil)
}

func (s *InvocationService) invokeTrustedPluginService(ctx context.Context, route router.InvokeResult, in InvocationInput, trace string) (map[string]interface{}, bool, error) {
	if s.catalog == nil || (route.Transport != "rest" && route.Transport != "http") {
		return nil, false, nil
	}
	view, err := s.catalog.GetCapability(ctx, in.CapabilityID, false)
	if err != nil {
		return nil, true, err
	}
	if len(view.Record.Protocols) == 0 {
		return nil, false, nil
	}
	var protocols []models.ProtocolBinding
	if json.Unmarshal(view.Record.Protocols, &protocols) != nil {
		return nil, true, pluginServiceError(503, "CAPABILITY_PROTOCOL_INVALID")
	}
	var binding *models.ProtocolBinding
	for i := range protocols {
		p := &protocols[i]
		if p.AuthType == PluginServiceAuthType && plugin_mgr.CatalogPluginEndpoint(view.Record.PluginID, p.Channel, p.Endpoint) == route.Endpoint && strings.EqualFold(p.Channel, route.Transport) {
			binding = p
			break
		}
	}
	if binding == nil {
		for _, p := range protocols {
			if p.AuthType == PluginServiceAuthType && ServiceCredential(ctx) {
				return nil, true, pluginServiceError(403, "CAPABILITY_SERVICE_BINDING_REQUIRED")
			}
		}
		return nil, false, nil
	}
	if !ServiceCredential(ctx) {
		return nil, true, pluginServiceError(401, "CAPABILITY_SERVICE_IDENTITY_REQUIRED")
	}
	if binding.ActorContext != "service_actor" || binding.ResourceScope != "tenant" || !strings.EqualFold(binding.Channel, "rest") || binding.Method == "" {
		return nil, true, pluginServiceError(503, "CAPABILITY_SERVICE_BINDING_INVALID")
	}
	if err := validateServiceTicketProtocols([]models.ProtocolBinding{*binding}); err != nil {
		return nil, true, pluginServiceError(503, "CAPABILITY_SERVICE_BINDING_INVALID")
	}
	// 独立服务入口只接收业务 body，拒绝调用方覆盖路由、请求头或可信身份。
	if len(in.Payload) != 1 || len(in.Context) != 0 {
		return nil, true, pluginServiceError(400, "CAPABILITY_SERVICE_PAYLOAD_INVALID")
	}
	body, ok := in.Payload["body"].(map[string]interface{})
	if !ok {
		return nil, true, pluginServiceError(400, "CAPABILITY_SERVICE_PAYLOAD_INVALID")
	}
	requestPath, err := FixedPluginServicePath(binding.Endpoint)
	if err != nil {
		return nil, true, pluginServiceError(503, "CAPABILITY_SERVICE_BINDING_INVALID")
	}
	if s.serviceIdentityIssuer == nil {
		return nil, true, pluginServiceError(503, "CAPABILITY_SERVICE_IDENTITY_UNAVAILABLE")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, true, pluginServiceError(400, "CAPABILITY_SERVICE_PAYLOAD_INVALID")
	}
	s.pluginTargetMu.RLock()
	resolver := s.pluginTargetResolver
	s.pluginTargetMu.RUnlock()
	if resolver == nil {
		return nil, true, pluginServiceError(503, "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE")
	}
	target, err := resolver(ctx, in.TenantUUID, view.Record.PluginID, view.Record.PluginVersion, requestPath)
	if err != nil {
		return nil, true, err
	}
	requestID := uuid.NewString()
	proof := ServiceInvocationBinding{TenantUUID: in.TenantUUID, TargetPluginID: view.Record.PluginID, CapabilityID: in.CapabilityID, ProviderVersion: view.Record.PluginVersion, CapabilitiesHash: view.Record.CapabilitiesHash, ProtocolHash: view.Record.ProtocolHash, Method: strings.ToUpper(binding.Method), Path: requestPath, BodySHA256: ServiceBodySHA256(raw), RequestID: requestID, TraceID: trace}
	ticket, err := s.serviceIdentityIssuer.IssueServiceInvocation(ctx, proof)
	if err != nil {
		return nil, true, err
	}
	defer s.serviceIdentityIssuer.DiscardServiceInvocation(context.WithoutCancel(ctx), ticket)
	req, err := http.NewRequestWithContext(ctx, proof.Method, target, bytes.NewReader(raw))
	if err != nil {
		return nil, true, pluginServiceError(503, "CAPABILITY_PLUGIN_RUNTIME_UNAVAILABLE")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(PluginServiceIdentityHeader, ticket)
	req.Header.Set("X-Request-ID", requestID)
	req.Header.Set("X-Trace-ID", trace)
	// 不携带原始 Key、Admin Bearer 或用户上下文头；禁止跨重定向传出票据。
	client := *s.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, true, pluginServiceError(502, "CAPABILITY_PLUGIN_TRANSPORT_FAILED")
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil || len(response) > 4*1024*1024 {
		return nil, true, pluginServiceError(502, "CAPABILITY_PLUGIN_RESPONSE_INVALID")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, true, newPluginServiceUpstreamError(resp.StatusCode, proof.Method, proof.Path, response)
	}
	var result map[string]interface{}
	if json.Unmarshal(response, &result) != nil || result == nil {
		return nil, true, pluginServiceError(502, "CAPABILITY_PLUGIN_RESPONSE_INVALID")
	}
	return result, true, nil
}

func ServiceBodySHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func FixedPluginServicePath(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || u.Path != value || !strings.HasPrefix(value, "/") || path.Clean(u.Path) != u.Path || strings.ContainsAny(value, "{}:*\\") || strings.HasPrefix(value, "//") {
		return "", errors.New("fixed plugin service path required")
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "_p" {
			return "", errors.New("host plugin proxy path is not a service endpoint")
		}
	}
	return u.Path, nil
}
