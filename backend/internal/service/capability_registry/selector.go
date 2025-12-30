package capability_registry

import (
	"context"
	"errors"
	"strings"

	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// Default fallback protocol when neither request nor policy declares a preference.
const defaultPreferredProtocol = "mcp"

var (
	// ErrSelectorUnavailable indicates the selector cannot process invocations.
	ErrSelectorUnavailable = errors.New("capability selector unavailable")
	// ErrSelectorTenantRequired indicates tenant identity is missing.
	ErrSelectorTenantRequired = errors.New("tenant uuid is required")
	// ErrSelectorCapabilityRequired indicates Selector cannot resolve capability mapping.
	ErrSelectorCapabilityRequired = errors.New("capability mapping not found")
	// ErrSelectorCapabilityForbidden indicates resolved capability is not granted to tenant.
	ErrSelectorCapabilityForbidden = errors.New("capability not granted for tenant")
	// ErrSelectorSafeModeActive indicates tenant safe-mode blocks invocations.
	ErrSelectorSafeModeActive = errors.New("tenant safe-mode active")
	// ErrSelectorToolGrantRequired indicates capability requires tool grants.
	ErrSelectorToolGrantRequired = errors.New("tool grant required for capability")
	// ErrSelectorFeatureFlagMissing indicates required feature flag is missing.
	ErrSelectorFeatureFlagMissing = errors.New("feature flag required for capability")
)

// CapabilityInvokeRequest is the normalized request Selector consumes regardless of protocol.
type CapabilityInvokeRequest struct {
	CapabilityID      string                 `json:"capability_id"`
	TenantUUID        string                 `json:"tenant_uuid"`
	Intent            string                 `json:"intent,omitempty"`
	ToolScope         string                 `json:"tool_scope,omitempty"`
	ToolGrantIDs      []string               `json:"tool_grant_ids,omitempty"`
	PreferredProtocol string                 `json:"preferred_protocol,omitempty"`
	IdempotencyKey    string                 `json:"idempotency_key,omitempty"`
	TraceID           string                 `json:"trace_id,omitempty"`
	Payload           map[string]interface{} `json:"payload"`
	Context           map[string]interface{} `json:"context"`
}

// CapabilityInvokeResponse captures Selector output shared by HTTP/gRPC/MCP handlers.
type CapabilityInvokeResponse struct {
	CapabilityID string                 `json:"capability_id"`
	TraceID      string                 `json:"trace_id"`
	Status       string                 `json:"status"`
	ProtocolUsed string                 `json:"protocol_used"`
	Result       map[string]interface{} `json:"result,omitempty"`
	FallbackUsed bool                   `json:"fallback_used"`
}

// SnapshotProviderFunc adapts plain functions to Selector snapshotProvider interface.
type SnapshotProviderFunc func(ctx context.Context, tenantUUID string, toolGrants []string) (SelectorPolicySnapshot, error)

// SelectorOptions describes dependencies required by Selector.
type SelectorOptions struct {
	Store      snapshotProvider
	Invoker    capabilityInvoker
	EventBus   event_bus.EventBus
	Metrics    *capmetrics.CapabilityRegistryMetrics
	Authorizer capabilityAuthorizer
}

type snapshotProvider interface {
	GetSnapshot(ctx context.Context, tenantUUID string, toolGrants []string) (SelectorPolicySnapshot, error)
}

// GetSnapshot implements snapshotProvider.
func (fn SnapshotProviderFunc) GetSnapshot(ctx context.Context, tenantUUID string, toolGrants []string) (SelectorPolicySnapshot, error) {
	if fn == nil {
		return SelectorPolicySnapshot{}, nil
	}
	return fn(ctx, tenantUUID, toolGrants)
}

type capabilityInvoker interface {
	Invoke(ctx context.Context, in InvocationInput) (InvocationResult, error)
}

// Selector orchestrates policy lookup + InvocationService to honor registry governance.
type Selector struct {
	store      snapshotProvider
	invoker    capabilityInvoker
	authorizer capabilityAuthorizer
	hooks      *selectorHooks
}

// NewSelector constructs a Selector instance.
func NewSelector(opts SelectorOptions) *Selector {
	if opts.Invoker == nil {
		panic("capability selector requires invoker")
	}
	return &Selector{
		store:      opts.Store,
		invoker:    opts.Invoker,
		authorizer: opts.Authorizer,
		hooks:      newSelectorHooks(opts.EventBus, opts.Metrics),
	}
}

// Invoke resolves the capability + protocol preference then delegates to InvocationService.
func (s *Selector) Invoke(ctx context.Context, req CapabilityInvokeRequest) (CapabilityInvokeResponse, error) {
	var resp CapabilityInvokeResponse
	if s == nil || s.invoker == nil {
		return resp, ErrSelectorUnavailable
	}
	reportReq := req
	tenant := strings.TrimSpace(req.TenantUUID)
	if tenant == "" {
		return s.fail(ctx, reportReq, resp, ErrSelectorTenantRequired)
	}
	reportReq.TenantUUID = tenant

	scope := strings.TrimSpace(req.ToolScope)
	if scope == "" {
		scope = extractStringFromContext(req.Context, "tool_scope")
	}
	intent := strings.TrimSpace(req.Intent)
	if intent == "" {
		intent = extractStringFromContext(req.Context, "intent")
	}
	reportReq.ToolScope = scope
	reportReq.Intent = intent

	var snapshot SelectorPolicySnapshot
	var err error
	if s.store != nil {
		snapshot, err = s.store.GetSnapshot(ctx, tenant, req.ToolGrantIDs)
		if err != nil {
			return s.fail(ctx, reportReq, resp, err)
		}
	}

	capabilityID := strings.TrimSpace(req.CapabilityID)
	resolvedScope := scope
	if capabilityID == "" {
		capabilityID, resolvedScope = resolveCapabilityFromMappings(snapshot.IntentMappings, intent, scope)
		if capabilityID == "" {
			return s.fail(ctx, reportReq, resp, ErrSelectorCapabilityRequired)
		}
	} else if s.store != nil && !capabilityAllowed(snapshot, capabilityID) {
		return s.fail(ctx, reportReq, resp, ErrSelectorCapabilityForbidden)
	}
	resp.CapabilityID = capabilityID
	reportReq.CapabilityID = capabilityID

	preferred := strings.TrimSpace(req.PreferredProtocol)
	if pref, ok := snapshot.PreferMatrix[capabilityID]; ok {
		if preferred == "" && strings.TrimSpace(pref.Prefer) != "" {
			preferred = strings.TrimSpace(pref.Prefer)
		}
	}
	if preferred == "" {
		preferred = defaultPreferredProtocol
	}
	reportReq.PreferredProtocol = preferred

	ctxPayload := cloneContextMap(req.Context)
	if ctxPayload == nil {
		ctxPayload = make(map[string]interface{})
	}
	if resolvedScope == "" {
		resolvedScope = defaultScopeKey
	}
	ctxPayload["tool_scope"] = resolvedScope
	reportReq.ToolScope = resolvedScope

	if s.authorizer != nil {
		if err := s.authorizer.AuthorizeInvocation(ctx, reportReq); err != nil {
			return s.fail(ctx, reportReq, resp, err)
		}
	}

	result, err := s.invoker.Invoke(ctx, InvocationInput{
		CapabilityID:      capabilityID,
		TenantUUID:        tenant,
		PreferredProtocol: preferred,
		IdempotencyKey:    strings.TrimSpace(req.IdempotencyKey),
		TraceID:           strings.TrimSpace(req.TraceID),
		Payload:           req.Payload,
		Context:           ctxPayload,
	})
	if err != nil {
		return resp, err
	}

	resp.TraceID = result.TraceID
	resp.Status = result.Status
	resp.ProtocolUsed = result.ProtocolUsed
	resp.FallbackUsed = result.FallbackUsed
	resp.Result = result.Result
	return resp, nil
}

func (s *Selector) fail(ctx context.Context, req CapabilityInvokeRequest, resp CapabilityInvokeResponse, err error) (CapabilityInvokeResponse, error) {
	if s != nil && s.hooks != nil {
		s.hooks.RecordFailure(ctx, req, err)
	}
	return resp, err
}

func resolveCapabilityFromMappings(mappings map[string]map[string]string, intent, scope string) (string, string) {
	if len(mappings) == 0 {
		return "", ""
	}
	intents := candidateIntents(intent)
	scopes := candidateScopes(scope)
	for _, intentKey := range intents {
		scopeMap, ok := mappings[intentKey]
		if !ok {
			continue
		}
		for _, scopeKey := range scopes {
			if capID := strings.TrimSpace(scopeMap[scopeKey]); capID != "" {
				return capID, scopeKey
			}
		}
	}
	return "", ""
}

func candidateIntents(intent string) []string {
	base := strings.TrimSpace(intent)
	if base == "" {
		return []string{defaultIntentKey}
	}
	if strings.EqualFold(base, defaultIntentKey) {
		return []string{defaultIntentKey}
	}
	return []string{base, defaultIntentKey}
}

func candidateScopes(scope string) []string {
	base := strings.TrimSpace(scope)
	if base == "" {
		return []string{defaultScopeKey}
	}
	if strings.EqualFold(base, defaultScopeKey) {
		return []string{defaultScopeKey}
	}
	return []string{base, defaultScopeKey}
}

func capabilityAllowed(snapshot SelectorPolicySnapshot, capabilityID string) bool {
	capabilityID = strings.TrimSpace(capabilityID)
	if capabilityID == "" {
		return false
	}
	if _, ok := snapshot.PreferMatrix[capabilityID]; ok {
		return true
	}
	for _, scopeMap := range snapshot.IntentMappings {
		for _, mapped := range scopeMap {
			if strings.EqualFold(strings.TrimSpace(mapped), capabilityID) {
				return true
			}
		}
	}
	return false
}

func extractStringFromContext(ctx map[string]interface{}, key string) string {
	if len(ctx) == 0 {
		return ""
	}
	if value, ok := ctx[key]; ok {
		if str, ok := value.(string); ok {
			return strings.TrimSpace(str)
		}
	}
	return ""
}

func cloneContextMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
