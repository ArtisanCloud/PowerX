package capability_registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// InvocationServiceOptions 配置能力调用服务。
type InvocationServiceOptions struct {
	Catalog     *RegistryService
	Router      *router.Service
	TraceRepo   *repo.InvocationTraceRepository
	EventRepo   *repo.CapabilityEventPublicationRepository
	EventBus    event_bus.EventBus
	Auditor     auditpkg.Auditor
	Metrics     *capmetrics.CapabilityRegistryMetrics
	Audit       *AuditService
	Clock       func() time.Time
	VersionLock VersionLock
	HTTPClient  *http.Client
	HTTPBaseURL string
	GRPCConn    *grpc.ClientConn
	ModelVerifier ModelKeyVerifier
}

// InvocationService 负责触发能力调用并记录追踪。
type InvocationService struct {
	catalog     *RegistryService
	router      *router.Service
	traces      *repo.InvocationTraceRepository
	audit       *AuditService
	versionLock VersionLock
	modelVerifier ModelKeyVerifier
	now         func() time.Time
	httpClient  *http.Client
	httpBaseURL string
	grpcConn    *grpc.ClientConn
}

// ModelKeyVerifier validates tenant-scoped model_key access.
type ModelKeyVerifier interface {
	VerifyModelKey(ctx context.Context, tenantUUID, env, modality, modelKey string) error
}

// InvocationInput 描述调用��求。
type InvocationInput struct {
	CapabilityID      string
	TenantUUID        string
	PreferredProtocol string
	IdempotencyKey    string
	TraceID           string
	Payload           map[string]interface{}
	Context           map[string]interface{}
}

// InvocationResult 描述调用结果。
type InvocationResult struct {
	TraceID      string
	Status       string
	ProtocolUsed string
	FallbackUsed bool
	Result       map[string]interface{}
}

// NewInvocationService 构造调用服务实例。
func NewInvocationService(opts InvocationServiceOptions) *InvocationService {
	if opts.Catalog == nil {
		panic("capability invocation service requires catalog service")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	audit := opts.Audit
	if audit == nil {
		audit = NewAuditService(AuditServiceOptions{
			EventRepo: opts.EventRepo,
			TraceRepo: opts.TraceRepo,
			EventBus:  opts.EventBus,
			Auditor:   opts.Auditor,
			Metrics:   opts.Metrics,
			Clock:     clock,
		})
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &InvocationService{
		catalog:     opts.Catalog,
		router:      opts.Router,
		traces:      opts.TraceRepo,
		audit:       audit,
		versionLock: opts.VersionLock,
		modelVerifier: opts.ModelVerifier,
		now:         clock,
		httpClient:  httpClient,
		httpBaseURL: strings.TrimSuffix(strings.TrimSpace(opts.HTTPBaseURL), "/"),
		grpcConn:    opts.GRPCConn,
	}
}

// VersionLock 描述版本锁执行器行为，用于校验 capabilities_hash。
type VersionLock interface {
	Enforce(ctx context.Context, tenantUUID, capabilityID, hash string) error
	IsUpgradeError(err error) bool
}

// ErrManualUpgradeRequired 表示能力调用前需要管理员确认升级。
var ErrManualUpgradeRequired = errors.New("capability manual upgrade required")

// Invoke 触发能力调用并记录追踪。
func (s *InvocationService) Invoke(ctx context.Context, in InvocationInput) (InvocationResult, error) {
	var result InvocationResult
	if s == nil {
		return result, errors.New("invocation service unavailable")
	}
	if s.router == nil {
		return result, errors.New("invocation router unavailable")
	}
	capabilityID := strings.TrimSpace(in.CapabilityID)
	tenantUUID := strings.TrimSpace(in.TenantUUID)
	if capabilityID == "" || tenantUUID == "" {
		return result, errors.New("capability_id and tenant_uuid are required")
	}

	traceID := strings.TrimSpace(in.TraceID)
	if traceID == "" {
		traceID = uuid.NewString()
	}
	result.TraceID = traceID

	view, err := s.catalog.GetCapability(ctx, capabilityID, false)
	if err != nil {
		return result, err
	}
	record := view.Record
	if record == nil {
		return result, fmt.Errorf("capability %s not found", capabilityID)
	}
	if !strings.EqualFold(strings.TrimSpace(record.Status), "published") {
		return result, fmt.Errorf("capability %s is not published", capabilityID)
	}
	if s.modelVerifier != nil && strings.HasPrefix(strings.ToLower(capabilityID), "com.corex.ai.") {
		modality := extractStringFromBody(in.Payload, "modality")
		modelKey := extractStringFromBody(in.Payload, "model_key")
		if modality == "" {
			modality = defaultModalityForCapability(capabilityID)
		}
		if modelKey == "" && strings.Contains(strings.ToLower(capabilityID), ".stream") {
			return result, fmt.Errorf("model_key required")
		}
		env := extractQueryString(in.Payload, "env")
		if err := s.modelVerifier.VerifyModelKey(ctx, tenantUUID, env, modality, modelKey); err != nil {
			return result, err
		}
	}

	if s.versionLock != nil && strings.TrimSpace(record.CapabilitiesHash) != "" {
		if err := s.versionLock.Enforce(ctx, tenantUUID, capabilityID, strings.TrimSpace(record.CapabilitiesHash)); err != nil {
			if s.versionLock.IsUpgradeError(err) {
				return result, fmt.Errorf("%w: %v", ErrManualUpgradeRequired, err)
			}
			return result, err
		}
	}

	payloadBytes, err := json.Marshal(in.Payload)
	if err != nil {
		return result, fmt.Errorf("invalid payload: %w", err)
	}
	stickyKey := extractStickyKey(in.Context)

	start := s.now()
	routerResult, invokeErr := s.router.Invoke(ctx, router.InvokeRequest{
		CapabilityID:      capabilityID,
		TenantUUID:        tenantUUID,
		Payload:           payloadBytes,
		StickyKey:         stickyKey,
		PreferredProtocol: strings.TrimSpace(in.PreferredProtocol),
	})
	latency := s.now().Sub(start)

	responseMap := decodeJSONPayload(routerResult.Payload)
	if invokeErr != nil {
		result.Status = "failed"
		result.ProtocolUsed = routerResult.Transport
		result.FallbackUsed = routerResult.FallbackUsed
		if s.audit != nil {
			s.audit.RecordInvocation(ctx, InvocationAuditInput{
				TraceID:           traceID,
				TenantUUID:        tenantUUID,
				PluginID:          record.PluginID,
				CapabilityID:      capabilityID,
				PreferredProtocol: in.PreferredProtocol,
				ProtocolUsed:      routerResult.Transport,
				FallbackUsed:      routerResult.FallbackUsed,
				Status:            result.Status,
				IdempotencyKey:    strings.TrimSpace(in.IdempotencyKey),
				RequestPayload:    in.Payload,
				ResponsePayload:   responseMap,
				ErrorSummary:      invokeErr.Error(),
				Latency:           latency,
			})
		}
		return result, invokeErr
	}

	result.Status = "completed"
	result.ProtocolUsed = routerResult.Transport
	result.FallbackUsed = routerResult.FallbackUsed

	if !routerResult.FallbackUsed {
		if payload, err := s.executeAdapterCall(ctx, routerResult, in, traceID); err != nil {
			result.Status = "failed"
			result.Result = responseMap
			if s.audit != nil {
				s.audit.RecordInvocation(ctx, InvocationAuditInput{
					TraceID:           traceID,
					TenantUUID:        tenantUUID,
					PluginID:          record.PluginID,
					CapabilityID:      capabilityID,
					PreferredProtocol: in.PreferredProtocol,
					ProtocolUsed:      routerResult.Transport,
					FallbackUsed:      routerResult.FallbackUsed,
					Status:            result.Status,
					IdempotencyKey:    strings.TrimSpace(in.IdempotencyKey),
					RequestPayload:    in.Payload,
					ResponsePayload:   responseMap,
					ErrorSummary:      err.Error(),
					Latency:           latency,
				})
			}
			return result, err
		} else if payload != nil {
			responseMap = payload
		}
	}

	result.Result = responseMap

	if s.audit != nil {
		s.audit.RecordInvocation(ctx, InvocationAuditInput{
			TraceID:           traceID,
			TenantUUID:        tenantUUID,
			PluginID:          record.PluginID,
			CapabilityID:      capabilityID,
			PreferredProtocol: in.PreferredProtocol,
			ProtocolUsed:      routerResult.Transport,
			FallbackUsed:      routerResult.FallbackUsed,
			Status:            result.Status,
			IdempotencyKey:    strings.TrimSpace(in.IdempotencyKey),
			RequestPayload:    in.Payload,
			ResponsePayload:   responseMap,
			Latency:           latency,
		})
	}

	return result, nil
}

// GetTrace 根据 trace_id 查询调用记录。
func (s *InvocationService) GetTrace(ctx context.Context, traceID string) (*models.InvocationTrace, error) {
	if s == nil || s.traces == nil {
		return nil, errors.New("invocation trace repository unavailable")
	}
	return s.traces.GetByTraceID(ctx, traceID)
}

func (s *InvocationService) executeAdapterCall(ctx context.Context, routerResult router.InvokeResult, in InvocationInput, traceID string) (map[string]interface{}, error) {
	transport := strings.ToLower(strings.TrimSpace(routerResult.Transport))
	if transport == "" {
		transport = strings.ToLower(strings.TrimSpace(in.PreferredProtocol))
	}

	switch transport {
	case "http", "rest":
		if s.httpClient == nil || s.httpBaseURL == "" {
			return nil, nil
		}
		restPayload, err := buildRESTInvokePayloadWithDefaults(in.Payload, routerResult.Endpoint, routerResult.Labels)
		if err != nil {
			return nil, err
		}
		return s.invokeREST(ctx, restPayload, traceID)
	case "grpc":
		if s.grpcConn == nil {
			return nil, nil
		}
		grpcPayload, err := buildGRPCInvokePayloadWithDefaults(in.Payload, routerResult.Endpoint, routerResult.Labels)
		if err != nil {
			return nil, err
		}
		return s.invokeGRPC(ctx, grpcPayload)
	default:
		return nil, nil
	}
}

type restInvokePayload struct {
	Method   string
	Endpoint string
	Headers  map[string]string
	Query    map[string]string
	Body     interface{}
}

type grpcInvokePayload struct {
	Service string
	Method  string
	Headers map[string]string
	Body    map[string]interface{}
}

func buildRESTInvokePayload(raw map[string]interface{}) (restInvokePayload, error) {
	return buildRESTInvokePayloadWithDefaults(raw, "", nil)
}

func buildRESTInvokePayloadWithDefaults(raw map[string]interface{}, defaultEndpoint string, labels map[string]string) (restInvokePayload, error) {
	if len(raw) == 0 {
		return restInvokePayload{}, errors.New("payload required for REST invocation")
	}
	method := strings.ToUpper(strings.TrimSpace(getString(raw["method"])))
	if method == "" {
		method = strings.ToUpper(strings.TrimSpace(getLabel(labels, "method")))
	}
	if method == "" {
		method = http.MethodGet
	}
	endpoint := strings.TrimSpace(getString(raw["endpoint"]))
	if endpoint == "" {
		endpoint = strings.TrimSpace(defaultEndpoint)
	}
	if endpoint == "" {
		return restInvokePayload{}, errors.New("payload.endpoint required for REST invocation")
	}
	headers := mapStringString(raw["headers"])
	query := mapStringString(raw["query"])

	var body interface{}
	if b, ok := raw["body"]; ok {
		body = mergeBodyWithTopLevel(raw, b)
	} else if method != http.MethodGet && method != http.MethodHead {
		body = stripEnvelopeKeys(raw)
	}

	return restInvokePayload{
		Method:   method,
		Endpoint: endpoint,
		Headers:  headers,
		Query:    query,
		Body:     body,
	}, nil
}

func buildGRPCInvokePayload(raw map[string]interface{}) (grpcInvokePayload, error) {
	return buildGRPCInvokePayloadWithDefaults(raw, "", nil)
}

func buildGRPCInvokePayloadWithDefaults(raw map[string]interface{}, endpoint string, labels map[string]string) (grpcInvokePayload, error) {
	if len(raw) == 0 {
		return grpcInvokePayload{}, errors.New("payload required for gRPC invocation")
	}
	service := strings.TrimSpace(getString(raw["endpoint"]))
	if service == "" {
		service = strings.TrimSpace(endpoint)
	}
	if service == "" {
		return grpcInvokePayload{}, errors.New("payload.endpoint required for gRPC invocation")
	}
	method := strings.TrimSpace(getString(raw["rpc"]))
	if method == "" {
		method = strings.TrimSpace(getLabel(labels, "rpc"))
	}
	if method == "" {
		return grpcInvokePayload{}, errors.New("payload.rpc required for gRPC invocation")
	}

	body := map[string]interface{}{}
	switch val := raw["body"].(type) {
	case map[string]interface{}:
		body = val
	case map[string]string:
		body = make(map[string]interface{}, len(val))
		for k, v := range val {
			body[k] = v
		}
	case nil:
		body = stripEnvelopeKeys(raw)
	default:
		return grpcInvokePayload{}, errors.New("payload.body for gRPC invocation must be an object")
	}

	return grpcInvokePayload{
		Service: service,
		Method:  method,
		Headers: mapStringString(raw["headers"]),
		Body:    body,
	}, nil
}

func (s *InvocationService) invokeREST(ctx context.Context, payload restInvokePayload, traceID string) (map[string]interface{}, error) {
	target := payload.Endpoint
	if !strings.HasPrefix(strings.ToLower(target), "http://") && !strings.HasPrefix(strings.ToLower(target), "https://") {
		target = strings.TrimRight(s.httpBaseURL, "/") + "/" + strings.TrimLeft(target, "/")
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint %s: %w", payload.Endpoint, err)
	}
	values := parsed.Query()
	for k, v := range payload.Query {
		values.Set(k, v)
	}
	parsed.RawQuery = values.Encode()

	var bodyReader io.Reader
	if payload.Body != nil && payload.Method != http.MethodGet && payload.Method != http.MethodHead {
		raw, err := json.Marshal(payload.Body)
		if err != nil {
			return nil, fmt.Errorf("encode REST body failed: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, payload.Method, parsed.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build REST request failed: %w", err)
	}
	hasContentType := false
	for k, v := range payload.Headers {
		if strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
		if strings.EqualFold(k, "Content-Type") {
			hasContentType = true
		}
	}
	if payload.Body != nil && !hasContentType {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if traceID != "" && req.Header.Get("X-Trace-Id") == "" {
		req.Header.Set("X-Trace-Id", traceID)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxy REST request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read REST response failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		snippet := string(respBytes)
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		return nil, fmt.Errorf("proxy REST %s %s failed: status=%d body=%s", payload.Method, parsed.Path, resp.StatusCode, snippet)
	}
	if len(respBytes) == 0 {
		return map[string]interface{}{
			"status": resp.Status,
		}, nil
	}
	var out interface{}
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return map[string]interface{}{
			"raw":    string(respBytes),
			"status": resp.Status,
		}, nil
	}
	if m, ok := out.(map[string]interface{}); ok {
		return m, nil
	}
	return map[string]interface{}{
		"value":  out,
		"status": resp.Status,
	}, nil
}

func (s *InvocationService) invokeGRPC(ctx context.Context, payload grpcInvokePayload) (map[string]interface{}, error) {
	if s.grpcConn == nil {
		return nil, errors.New("grpc proxy unavailable")
	}
	serviceName := protoreflect.FullName(payload.Service)
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(serviceName)
	if err != nil {
		return nil, fmt.Errorf("grpc service %s not registered: %w", payload.Service, err)
	}
	serviceDesc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%s is not a gRPC service", payload.Service)
	}
	methodDesc := serviceDesc.Methods().ByName(protoreflect.Name(payload.Method))
	if methodDesc == nil {
		return nil, fmt.Errorf("grpc method %s not found in %s", payload.Method, payload.Service)
	}

	reqMsg := dynamicpb.NewMessage(methodDesc.Input())
	requestBytes, err := json.Marshal(payload.Body)
	if err != nil {
		return nil, fmt.Errorf("encode gRPC body failed: %w", err)
	}
	if err := protojson.Unmarshal(requestBytes, reqMsg); err != nil {
		return nil, fmt.Errorf("decode gRPC body failed: %w", err)
	}

	respMsg := dynamicpb.NewMessage(methodDesc.Output())
	md := metadata.New(nil)
	for k, v := range payload.Headers {
		if strings.TrimSpace(v) == "" {
			continue
		}
		md.Append(k, v)
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	fullMethod := fmt.Sprintf("/%s/%s", payload.Service, payload.Method)
	if err := s.grpcConn.Invoke(ctx, fullMethod, reqMsg, respMsg); err != nil {
		return nil, fmt.Errorf("grpc invoke %s failed: %w", fullMethod, err)
	}

	respBytes, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(respMsg)
	if err != nil {
		return nil, fmt.Errorf("encode gRPC response failed: %w", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("decode gRPC response failed: %w", err)
	}
	return out, nil
}

func mapStringString(v interface{}) map[string]string {
	result := make(map[string]string)
	switch typed := v.(type) {
	case map[string]interface{}:
		for key, value := range typed {
			result[key] = fmt.Sprint(value)
		}
	case map[string]string:
		for key, value := range typed {
			result[key] = value
		}
	}
	return result
}

func getString(v interface{}) string {
	switch typed := v.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case json.Number:
		return typed.String()
	default:
		if typed == nil {
			return ""
		}
		return fmt.Sprint(typed)
	}
}

func extractString(m map[string]interface{}, key string) string {
	if len(m) == 0 {
		return ""
	}
	if value, ok := m[key]; ok {
		return strings.TrimSpace(getString(value))
	}
	return ""
}

func extractStringFromBody(payload map[string]interface{}, key string) string {
	if len(payload) == 0 {
		return ""
	}
	body, ok := payload["body"]
	if !ok || body == nil {
		return ""
	}
	switch typed := body.(type) {
	case map[string]interface{}:
		return extractString(typed, key)
	default:
		return ""
	}
}

func extractQueryString(payload map[string]interface{}, key string) string {
	if len(payload) == 0 {
		return ""
	}
	query, ok := payload["query"]
	if !ok || query == nil {
		return ""
	}
	switch typed := query.(type) {
	case map[string]interface{}:
		return extractString(typed, key)
	default:
		return ""
	}
}

func getLabel(labels map[string]string, key string) string {
	if len(labels) == 0 {
		return ""
	}
	return strings.TrimSpace(labels[key])
}

func stripEnvelopeKeys(raw map[string]interface{}) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "method", "endpoint", "headers", "query", "body", "rpc", "stream":
			continue
		default:
			result[k] = v
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mergeBodyWithTopLevel(raw map[string]interface{}, body interface{}) interface{} {
	if len(raw) == 0 || body == nil {
		return body
	}
	bodyMap, ok := toStringAnyMap(body)
	if !ok {
		return body
	}
	merged := make(map[string]interface{}, len(bodyMap)+len(raw))
	for k, v := range bodyMap {
		merged[k] = v
	}
	for k, v := range raw {
		key := strings.ToLower(strings.TrimSpace(k))
		switch key {
		case "method", "endpoint", "headers", "query", "body", "rpc", "stream":
			continue
		}
		if _, exists := merged[k]; exists {
			continue
		}
		merged[k] = v
	}
	return merged
}

func toStringAnyMap(v interface{}) (map[string]interface{}, bool) {
	switch typed := v.(type) {
	case map[string]interface{}:
		return typed, true
	default:
		return nil, false
	}
}

func defaultModalityForCapability(capabilityID string) string {
	lower := strings.ToLower(strings.TrimSpace(capabilityID))
	if strings.Contains(lower, "llm") {
		return "llm"
	}
	if strings.Contains(lower, "image") {
		return "image"
	}
	if strings.Contains(lower, "video") {
		return "video"
	}
	if strings.Contains(lower, "tts") {
		return "audio_tts"
	}
	if strings.Contains(lower, "embedding") || strings.Contains(lower, "embeddings") {
		return "embedding"
	}
	if strings.Contains(lower, "multimodal") {
		return "mixed"
	}
	return ""
}

func extractStickyKey(ctx map[string]interface{}) string {
	if len(ctx) == 0 {
		return ""
	}
	const workflowKey = "workflow_instance_id"
	if value, ok := ctx[workflowKey]; ok {
		if str, ok := value.(string); ok {
			return strings.TrimSpace(str)
		}
	}
	return ""
}

func decodeJSONPayload(data []byte) map[string]interface{} {
	if len(data) == 0 {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
