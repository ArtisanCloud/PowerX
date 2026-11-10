package plugin_identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	plugin "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/stsproof"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const ServiceInvocationSchema = "powerx.iam.service-invocation/v1"
const ServiceInvocationQuerySchema = "powerx.iam.service-invocation-query/v1"

var serviceTicketPattern = regexp.MustCompile(`^pxsvc_v1_[A-Za-z0-9_-]{43}$`)
var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type ServiceInvocationQuery struct {
	Schema            string `json:"schema"`
	InvocationTicket  string `json:"invocation_ticket"`
	CapabilityID      string `json:"capability_id"`
	CapabilityVersion string `json:"capability_version"`
	Method            string `json:"method"`
	Path              string `json:"path"`
	BodySHA256        string `json:"body_sha256"`
}

type ServiceInvocationResult struct {
	Schema         string `json:"schema"`
	ActorType      string `json:"actor_type"`
	CallerSubject  string `json:"caller_subject"`
	CallerPluginID string `json:"caller_plugin_id,omitempty"`
	cap.ServiceInvocationBinding
	ExpiresAt time.Time `json:"expires_at"`
}

type serviceInvocationProof struct {
	Result              ServiceInvocationResult `json:"result"`
	CallerClaims        reqctx.CoreXClaims      `json:"caller_claims"`
	CallerAPIKeyHash    string                  `json:"caller_api_key_hash,omitempty"`
	RegistrationVersion uint64                  `json:"registration_version"`
}

func (s *Service) serviceInvocationKey(ticket string) string {
	return "iam:service-invocation:v1:" + s.opts.Namespace + ":" + hash(ticket)
}

// IssueServiceInvocation 只能由进程内能力执行器调用；不提供客户端签发接口。
// 随机票据由 Core 持有权威内容，共享 JWT secret 无法伪造这些内容。
func (s *Service) IssueServiceInvocation(ctx context.Context, binding cap.ServiceInvocationBinding) (string, error) {
	if !s.ready() {
		return "", failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
	}
	c := reqctx.GetClaims(ctx)
	if !cap.ServiceCredential(ctx) || c == nil || c.IsRoot || c.UserUUID != "" || c.MemberUUID != "" {
		return "", failure(401, "IAM_SERVICE_INVOCATION_CALLER_INVALID")
	}
	if err := s.requireIssuedServiceCredential(ctx, c); err != nil {
		return "", err
	}
	if binding.TenantUUID != canonical(reqctx.GetTenantUUID(ctx)) || binding.TenantUUID == "" {
		return "", failure(403, "IAM_SERVICE_INVOCATION_TENANT_FORBIDDEN")
	}
	if !sha256Pattern.MatchString(binding.BodySHA256) || binding.TraceID == "" || binding.RequestID == "" {
		return "", failure(400, "IAM_SERVICE_INVOCATION_INVALID_ARGUMENT")
	}
	access, err := cap.NewGrantStatusService(s.db).CurrentAccess(ctx)
	if err != nil {
		return "", err
	}
	if err = access.Require(binding.CapabilityID); err != nil {
		return "", err
	}
	reg, err := s.serviceInvocationBinding(ctx, &binding)
	if err != nil {
		return "", err
	}
	expiry := s.clock().UTC().Add(time.Minute)
	if c.ExpiresAt != nil && c.ExpiresAt.Time.Before(expiry) {
		expiry = c.ExpiresAt.Time
	}
	if !s.clock().Before(expiry) {
		return "", failure(401, "IAM_SERVICE_INVOCATION_CALLER_INVALID")
	}
	callerPlugin := ""
	if len(access.PluginOwners) == 1 {
		callerPlugin = access.PluginOwners[0]
	}
	proof := serviceInvocationProof{Result: ServiceInvocationResult{Schema: ServiceInvocationSchema, ActorType: "service_actor", CallerSubject: access.Subject, CallerPluginID: callerPlugin, ServiceInvocationBinding: binding, ExpiresAt: expiry}, CallerClaims: *c, CallerAPIKeyHash: reqctx.AuthenticatedAPIKeyHash(ctx), RegistrationVersion: reg.Version}
	random := make([]byte, 32)
	if _, err = rand.Read(random); err != nil {
		return "", failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
	}
	ticket := "pxsvc_v1_" + base64.RawURLEncoding.EncodeToString(random)
	raw, _ := json.Marshal(proof)
	if err = s.store.Set(ctx, s.serviceInvocationKey(ticket), raw, expiry.Sub(s.clock())); err != nil {
		return "", failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
	}
	return ticket, nil
}

func (s *Service) DiscardServiceInvocation(ctx context.Context, ticket string) {
	if s.ready() && serviceTicketPattern.MatchString(ticket) {
		_ = s.store.Delete(ctx, s.serviceInvocationKey(ticket))
	}
}

// ResolveServiceInvocation 用目标插件自身凭据回查，并实时复核源调用方撤权。
// 普通 Admin JWT、仅有用户身份票据、普通身份请求头都不能进入这个平面。
func (s *Service) ResolveServiceInvocation(ctx context.Context, q ServiceInvocationQuery) (ServiceInvocationResult, error) {
	var empty ServiceInvocationResult
	if !s.ready() {
		return empty, failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
	}
	tenant, err := s.serviceActor(ctx)
	if err != nil {
		return empty, err
	}
	if err = s.requireIssuedServiceCredential(ctx, reqctx.GetClaims(ctx)); err != nil {
		return empty, err
	}
	if q.Schema != ServiceInvocationQuerySchema || q.CapabilityID == "" || q.CapabilityVersion == "" || q.Method == "" || q.Path == "" || !sha256Pattern.MatchString(q.BodySHA256) {
		return empty, failure(400, "IAM_SERVICE_INVOCATION_INVALID_ARGUMENT")
	}
	if !serviceTicketPattern.MatchString(q.InvocationTicket) {
		return empty, failure(401, "IAM_SERVICE_INVOCATION_PROOF_INVALID")
	}
	raw, err := s.store.Get(ctx, s.serviceInvocationKey(q.InvocationTicket))
	if err != nil {
		return empty, failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
	}
	if len(raw) == 0 {
		return empty, failure(401, "IAM_SERVICE_INVOCATION_PROOF_EXPIRED")
	}
	var p serviceInvocationProof
	if json.Unmarshal(raw, &p) != nil || p.Result.Schema != ServiceInvocationSchema {
		return empty, failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
	}
	r := p.Result
	if !s.clock().Before(r.ExpiresAt) {
		return empty, failure(401, "IAM_SERVICE_INVOCATION_PROOF_EXPIRED")
	}
	if r.TenantUUID != tenant {
		return empty, failure(403, "IAM_SERVICE_INVOCATION_TENANT_FORBIDDEN")
	}
	receiver, err := cap.NewGrantStatusService(s.db).CurrentAccess(ctx)
	if err != nil {
		return empty, err
	}
	if !receiver.OwnsPlugin(r.TargetPluginID) {
		return empty, failure(403, "IAM_SERVICE_INVOCATION_TARGET_FORBIDDEN")
	}
	if q.CapabilityID != r.CapabilityID || q.CapabilityVersion != r.CapabilityVersion || strings.ToUpper(q.Method) != r.Method || q.Path != r.Path || q.BodySHA256 != r.BodySHA256 {
		return empty, failure(403, "IAM_SERVICE_INVOCATION_REQUEST_MISMATCH")
	}
	callerCtx := reqctx.WithClaims(reqctx.WithTenantUUID(ctx, r.TenantUUID), &p.CallerClaims)
	callerCtx = reqctx.WithAuthenticatedAPIKeyHash(callerCtx, p.CallerAPIKeyHash)
	if err = s.requireIssuedServiceCredential(callerCtx, &p.CallerClaims); err != nil {
		return empty, err
	}
	caller, err := cap.NewGrantStatusService(s.db).CurrentAccess(callerCtx)
	if err != nil {
		return empty, err
	}
	if caller.Subject != r.CallerSubject {
		return empty, failure(403, "IAM_SERVICE_INVOCATION_CALLER_INVALID")
	}
	if err = caller.Require(r.CapabilityID); err != nil {
		return empty, err
	}
	if r.CallerPluginID != "" && !caller.OwnsPlugin(r.CallerPluginID) {
		return empty, failure(403, "IAM_SERVICE_INVOCATION_CALLER_INVALID")
	}
	binding := r.ServiceInvocationBinding
	reg, err := s.serviceInvocationBinding(ctx, &binding)
	if err != nil {
		return empty, err
	}
	if reg.Version != p.RegistrationVersion || binding.CapabilityVersion != r.CapabilityVersion {
		return empty, failure(403, "IAM_SERVICE_INVOCATION_VERSION_CHANGED")
	}
	return r, nil
}

func (s *Service) requireIssuedServiceCredential(ctx context.Context, c *reqctx.CoreXClaims) error {
	if c != nil && c.Issuer == "powerx-sts" {
		if err := stsproof.Verify(ctx, s.store, c); err != nil {
			if errors.Is(err, stsproof.ErrUnavailable) {
				return failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
			}
			return failure(401, "IAM_SERVICE_INVOCATION_STS_NOT_ISSUED")
		}
	}
	return nil
}

func (s *Service) serviceInvocationBinding(ctx context.Context, b *cap.ServiceInvocationBinding) (cm.CapabilityRegistration, error) {
	var reg cm.CapabilityRegistration
	if !pluginPattern.MatchString(b.TargetPluginID) || !strings.HasPrefix(b.CapabilityID, b.TargetPluginID+".") || b.ProviderVersion == "" || b.CapabilitiesHash == "" || b.ProtocolHash == "" {
		return reg, failure(403, "IAM_SERVICE_INVOCATION_BINDING_FORBIDDEN")
	}
	var tenant tenantmodel.Tenant
	if err := s.db.WithContext(ctx).Where("uuid = ?", b.TenantUUID).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return reg, failure(403, "IAM_SERVICE_INVOCATION_TENANT_FORBIDDEN")
		}
		return reg, failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
	}
	if tenant.Status != tenantmodel.TenantStatusActive {
		return reg, failure(403, "IAM_SERVICE_INVOCATION_TENANT_FORBIDDEN")
	}
	var record cm.CapabilityRecord
	if err := s.db.WithContext(ctx).Where("capability_id = ? AND plugin_id = ? AND status = ?", b.CapabilityID, b.TargetPluginID, "published").First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return reg, failure(403, "IAM_SERVICE_INVOCATION_CAPABILITY_FORBIDDEN")
		}
		return reg, failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
	}
	if record.PluginVersion != b.ProviderVersion || record.CapabilitiesHash != b.CapabilitiesHash || record.ProtocolHash != b.ProtocolHash {
		return reg, failure(403, "IAM_SERVICE_INVOCATION_VERSION_CHANGED")
	}
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND capability_id = ?", b.TenantUUID, b.CapabilityID).Order("version DESC").First(&reg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return reg, failure(403, "IAM_SERVICE_INVOCATION_REGISTRATION_FORBIDDEN")
		}
		return reg, failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
	}
	if reg.Status != "published" || reg.ContractRef == "" {
		return reg, failure(403, "IAM_SERVICE_INVOCATION_REGISTRATION_FORBIDDEN")
	}
	b.CapabilityVersion = reg.ContractRef
	var protocols []cm.ProtocolBinding
	if json.Unmarshal(record.Protocols, &protocols) != nil {
		return reg, failure(503, "IAM_SERVICE_INVOCATION_UNAVAILABLE")
	}
	matched := false
	for _, binding := range protocols {
		if binding.AuthType == cap.PluginServiceAuthType && binding.ActorContext == "service_actor" && binding.ResourceScope == "tenant" && binding.Channel == "rest" && binding.Method == b.Method && binding.Endpoint == b.Path {
			matched = true
		}
	}
	if !matched {
		return reg, failure(403, "IAM_SERVICE_INVOCATION_BINDING_FORBIDDEN")
	}
	if _, err := cap.FixedPluginServicePath(b.Path); err != nil {
		return reg, failure(403, "IAM_SERVICE_INVOCATION_BINDING_FORBIDDEN")
	}
	if _, err := uuid.Parse(b.TenantUUID); err != nil {
		return reg, failure(403, "IAM_SERVICE_INVOCATION_TENANT_FORBIDDEN")
	}
	if err := plugin.NewTenantPluginInstanceService(s.db).RequireEnabled(ctx, b.TenantUUID, b.TargetPluginID); err != nil {
		return reg, err
	}
	return reg, nil
}
