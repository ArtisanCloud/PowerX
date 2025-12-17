package capability_registry

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	caperrdto "github.com/ArtisanCloud/PowerX/internal/dto/capability_registry"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/capability_registrydto"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type tenantHandler struct {
	catalog  *capservice.RegistryService
	invoker  *capservice.InvocationService
	selector *capservice.Selector
}

func newTenantHandler(deps *shared.Deps) *tenantHandler {
	if deps == nil || deps.CapabilityCatalogSvc == nil {
		return nil
	}
	invocationSvc := deps.CapabilityInvocationSvc
	if invocationSvc == nil {
		var traceRepo *repo.InvocationTraceRepository
		var eventRepo *repo.CapabilityEventPublicationRepository
		if deps.DB != nil {
			traceRepo = repo.NewInvocationTraceRepository(deps.DB)
			eventRepo = repo.NewCapabilityEventPublicationRepository(deps.DB)
		}
		invocationSvc = capservice.NewInvocationService(capservice.InvocationServiceOptions{
			Catalog:     deps.CapabilityCatalogSvc,
			Router:      deps.RouterSvc,
			TraceRepo:   traceRepo,
			EventRepo:   eventRepo,
			EventBus:    deps.EventBus,
			Auditor:     deps.Auditor,
			VersionLock: deps.VersionLockStore,
		})
	}
	selector := deps.CapabilitySelector
	if selector == nil && invocationSvc != nil {
		selector = capservice.NewSelector(capservice.SelectorOptions{
			Invoker:  invocationSvc,
			EventBus: deps.EventBus,
		})
	}
	return &tenantHandler{
		catalog:  deps.CapabilityCatalogSvc,
		invoker:  invocationSvc,
		selector: selector,
	}
}

func (h *tenantHandler) ListCapabilities(c *gin.Context) {
	if h == nil || h.catalog == nil {
		caperrdto.RespondError(c, caperrdto.ErrUnavailable, nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		respondTenantIdentityError(c, err)
		return
	}

	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "50"), 50)
	if page <= 0 || pageSize <= 0 {
		caperrdto.RespondError(c, caperrdto.ErrInvalidRequest.WithHint("page and page_size must be positive"), nil)
		return
	}

	opts := capservice.CapabilityListOptions{
		PluginID:                 strings.TrimSpace(c.Query("plugin_id")),
		Intent:                   strings.TrimSpace(c.Query("intent")),
		ToolScope:                strings.TrimSpace(c.Query("channel")),
		TenantUUID:               tenantUUID,
		Status:                   []string{"published"},
		Limit:                    pageSize,
		Offset:                   (page - 1) * pageSize,
		IncludeWorkflowTemplates: false,
		IncludeTotal:             true,
	}
	if protocol := strings.TrimSpace(c.Query("protocol")); protocol != "" {
		opts.Protocol = protocol
	}

	records, total, err := h.catalog.ListCapabilities(c.Request.Context(), opts)
	if err != nil {
		caperrdto.RespondError(c, caperrdto.ErrInternal, err)
		return
	}

	items := make([]capability_registrydto.CapabilityRecordDTO, 0, len(records))
	for _, view := range records {
		items = append(items, capability_registrydto.CapabilityViewToDTO(view, false))
	}

	dto.ResponseList(c, items, &dto.PaginationResponse{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

func (h *tenantHandler) InvokeCapability(c *gin.Context) {
	if h == nil || h.selector == nil {
		caperrdto.RespondError(c, caperrdto.ErrUnavailable, nil)
		return
	}
	var req capabilityInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		caperrdto.RespondError(c, caperrdto.ErrInvalidRequest, err)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		respondTenantIdentityError(c, err)
		return
	}
	bodyTenant := strings.TrimSpace(req.TenantUUID)
	if bodyTenant != "" {
		canonical, err := reqctx.CanonicalTenantUUID(bodyTenant)
		if err != nil {
			respondTenantIdentityError(c, err)
			return
		}
		if canonical != tenantUUID {
			caperrdto.RespondError(c, caperrdto.ErrTenantMismatch, nil)
			return
		}
	}
	if strings.TrimSpace(req.CapabilityID) == "" && strings.TrimSpace(req.Intent) == "" {
		caperrdto.RespondError(c, caperrdto.ErrInvalidRequest.WithHint("capability_id or intent is required"), nil)
		return
	}

	payload := req.Payload
	if payload == nil {
		payload = map[string]interface{}{}
	}
	contextMap := cloneContext(req.Context)

	result, err := h.selector.Invoke(c.Request.Context(), capservice.CapabilityInvokeRequest{
		CapabilityID:      strings.TrimSpace(req.CapabilityID),
		Intent:            strings.TrimSpace(req.Intent),
		ToolScope:         strings.TrimSpace(req.ToolScope),
		TenantUUID:        tenantUUID,
		ToolGrantIDs:      normalizeToolGrantIDs(req.ToolGrantIDs),
		PreferredProtocol: strings.TrimSpace(req.PreferredProtocol),
		IdempotencyKey:    strings.TrimSpace(req.IdempotencyKey),
		TraceID:           strings.TrimSpace(req.TraceID),
		Payload:           payload,
		Context:           contextMap,
	})
	if err != nil {
		template := selectInvokeErrorTemplate(err)
		caperrdto.RespondError(c, template, err)
		return
	}

	resp := capabilityInvokeResponse{
		TraceID:      result.TraceID,
		Status:       result.Status,
		ProtocolUsed: result.ProtocolUsed,
		FallbackUsed: result.FallbackUsed,
		Result:       result.Result,
	}
	dto.ResponseSuccess(c, resp)
}

func (h *tenantHandler) GetInvocation(c *gin.Context) {
	if h == nil || h.invoker == nil {
		caperrdto.RespondError(c, caperrdto.ErrUnavailable, nil)
		return
	}
	traceID := strings.TrimSpace(c.Param("traceId"))
	if traceID == "" {
		caperrdto.RespondError(c, caperrdto.ErrInvalidRequest.WithHint("trace_id is required"), nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		respondTenantIdentityError(c, err)
		return
	}
	record, err := h.invoker.GetTrace(c.Request.Context(), traceID)
	if err != nil {
		template := caperrdto.ErrInternal
		if errors.Is(err, repo.ErrInvocationTraceNotFound) || errors.Is(err, repo.ErrCapabilityRecordNotFound) {
			template = caperrdto.ErrNotFound
		}
		caperrdto.RespondError(c, template, err)
		return
	}
	if !strings.EqualFold(record.TenantUUID, tenantUUID) {
		caperrdto.RespondError(c, caperrdto.ErrNotFound, nil)
		return
	}

	resp := invocationStatusResponse{
		TraceID:        record.TraceID,
		TenantUUID:     record.TenantUUID,
		CapabilityID:   record.CapabilityID,
		ProtocolUsed:   record.ProtocolUsed,
		FallbackUsed:   record.FallbackUsed,
		Status:         record.Status,
		Error:          buildErrorObject(record.ErrorSummary),
		LatencyMS:      record.LatencyMS,
		EventPublished: record.EventPublished,
	}
	dto.ResponseSuccess(c, resp)
}

type capabilityInvokeRequest struct {
	CapabilityID      string                 `json:"capability_id"`
	Intent            string                 `json:"intent"`
	ToolScope         string                 `json:"tool_scope"`
	TenantUUID        string                 `json:"tenant_uuid"`
	IdempotencyKey    string                 `json:"idempotency_key"`
	PreferredProtocol string                 `json:"preferred_protocol"`
	TraceID           string                 `json:"trace_id"`
	ToolGrantIDs      []string               `json:"tool_grant_ids"`
	Payload           map[string]interface{} `json:"payload"`
	Context           map[string]interface{} `json:"context"`
}

type capabilityInvokeResponse struct {
	TraceID      string                 `json:"trace_id"`
	Status       string                 `json:"status"`
	ProtocolUsed string                 `json:"protocol_used"`
	Result       map[string]interface{} `json:"result,omitempty"`
	FallbackUsed bool                   `json:"fallback_used"`
}

type invocationStatusResponse struct {
	TraceID        string            `json:"trace_id"`
	TenantUUID     string            `json:"tenant_uuid"`
	CapabilityID   string            `json:"capability_id"`
	ProtocolUsed   string            `json:"protocol_used"`
	FallbackUsed   bool              `json:"fallback_used"`
	Status         string            `json:"status"`
	Error          map[string]string `json:"error,omitempty"`
	LatencyMS      int               `json:"latency_ms"`
	EventPublished bool              `json:"event_published"`
}

func tenantUUIDFromRequest(c *gin.Context) (string, error) {
	if c == nil {
		return "", reqctx.ErrTenantUUIDMissing
	}
	if tenant := strings.TrimSpace(reqctx.TenantUUIDFromGin(c)); tenant != "" {
		return reqctx.CanonicalTenantUUID(tenant)
	}
	for _, candidate := range []string{
		c.GetHeader("X-Tenant-UUID"),
		c.GetHeader("X-PowerX-Tenant"),
		c.Query("tenant_uuid"),
	} {
		value := strings.TrimSpace(candidate)
		if value == "" {
			continue
		}
		return reqctx.CanonicalTenantUUID(value)
	}
	return "", reqctx.ErrTenantUUIDMissing
}

func respondTenantIdentityError(c *gin.Context, err error) {
	if errors.Is(err, reqctx.ErrTenantUUIDMissing) {
		caperrdto.RespondError(c, caperrdto.ErrTenantUUIDMissing, err)
		return
	}
	caperrdto.RespondError(c, caperrdto.ErrTenantUUIDInvalid, err)
}

func parsePositiveInt(value string, fallback int) int {
	i, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || i <= 0 {
		return fallback
	}
	return i
}

func buildErrorObject(summary string) map[string]string {
	if strings.TrimSpace(summary) == "" {
		return nil
	}
	return map[string]string{
		"code":    "invocation_failed",
		"message": summary,
	}
}

func selectInvokeErrorTemplate(err error) caperrdto.ErrorTemplate {
	switch {
	case errors.Is(err, capservice.ErrManualUpgradeRequired):
		return caperrdto.ErrVersionLocked
	case errors.Is(err, capservice.ErrSelectorCapabilityRequired):
		return caperrdto.ErrNotFound.WithHint("capability not found or not published for tenant")
	case errors.Is(err, capservice.ErrSelectorCapabilityForbidden):
		return caperrdto.ErrCapabilityForbidden
	case errors.Is(err, capservice.ErrSelectorTenantRequired):
		return caperrdto.ErrTenantUUIDMissing
	case errors.Is(err, capservice.ErrSelectorSafeModeActive):
		return caperrdto.ErrSafeModeActive
	case errors.Is(err, capservice.ErrSelectorToolGrantRequired):
		return caperrdto.ErrToolGrantMissing
	case errors.Is(err, capservice.ErrSelectorFeatureFlagMissing):
		return caperrdto.ErrFeatureFlagMissing
	case errors.Is(err, capservice.ErrSelectorUnavailable):
		return caperrdto.ErrUnavailable
	default:
		return caperrdto.ErrInvokeFailed
	}
}

func normalizeToolGrantIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := unique[key]; exists {
			continue
		}
		unique[key] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	sort.Strings(result)
	return result
}

func cloneContext(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
