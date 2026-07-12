package capability_registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	capability_registrydto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
)

const capabilityResolvePageSize = 200

type tenantHandler struct {
	catalog      *capservice.RegistryService
	invoker      *capservice.InvocationService
	selector     *capservice.Selector
	skillAdapter *skillservice.AdapterService
	memberSvc    *iamsvc.MemberService
	httpClient   *http.Client
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
			CoreInvoker: customersvc.NewCapabilityInvoker(customersvc.NewAccountService(deps.DB)),
		})
	}
	selector := deps.CapabilitySelector
	if selector == nil && invocationSvc != nil {
		selector = capservice.NewSelector(capservice.SelectorOptions{
			Invoker:  invocationSvc,
			EventBus: deps.EventBus,
		})
	}
	var skillAdapter *skillservice.AdapterService
	var memberSvc *iamsvc.MemberService
	if deps.DB != nil {
		skillRegistryRepo := skillrepo.NewSkillRegistryRepository(deps.DB)
		skillBindingRepo := skillrepo.NewSkillCapabilityBindingRepository(deps.DB)
		skillTraceRepo := skillrepo.NewSkillExecutionTraceRepository(deps.DB)
		skillAuditRepo := skillrepo.NewSkillLifecycleAuditRepository(deps.DB)
		skillInvokeSvc := skillservice.NewInvokeService(skillRegistryRepo, skillservice.NewAuditTraceService(skillTraceRepo, skillAuditRepo))
		skillAdapter = skillservice.NewAdapterService(skillInvokeSvc, skillBindingRepo).
			WithSourcePolicyResolver(skillservice.NewDBSourcePolicyResolver(deps.DB))
		memberSvc = iamsvc.NewMemberService(deps.DB)
	}

	return &tenantHandler{
		catalog:      deps.CapabilityCatalogSvc,
		invoker:      invocationSvc,
		selector:     selector,
		skillAdapter: skillAdapter,
		memberSvc:    memberSvc,
		httpClient:   &http.Client{},
	}
}

func (h *tenantHandler) ListCapabilities(c *gin.Context) {
	if h == nil || h.catalog == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
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
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("page and page_size must be positive"), nil)
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
	if sourceParam := strings.TrimSpace(c.Query("source")); sourceParam != "" {
		source, err := capservice.NormalizeCapabilitySource(sourceParam)
		if err != nil {
			capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("source must be corex or plugin"), err)
			return
		}
		opts.Source = source
	}

	records, total, err := h.catalog.ListCapabilities(c.Request.Context(), opts)
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInternal, err)
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

type capabilityResolveMatch struct {
	CapabilityID      string `json:"capability_id"`
	PluginID          string `json:"plugin_id"`
	Source            string `json:"source"`
	Protocol          string `json:"protocol"`
	Method            string `json:"method"`
	PatternEndpoint   string `json:"pattern_endpoint"`
	RequestedEndpoint string `json:"requested_endpoint"`
	Title             string `json:"title,omitempty"`
}

func (h *tenantHandler) ResolveCapability(c *gin.Context) {
	if h == nil || h.catalog == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		respondTenantIdentityError(c, err)
		return
	}

	method := strings.ToUpper(strings.TrimSpace(c.Query("method")))
	endpoint := normalizeResolveEndpoint(c.Query("endpoint"))
	if method == "" || endpoint == "" {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("method and endpoint are required"), nil)
		return
	}

	source := capservice.CapabilitySourceCoreX
	if rawSource := strings.TrimSpace(c.Query("source")); rawSource != "" {
		source, err = capservice.NormalizeCapabilitySource(rawSource)
		if err != nil {
			capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("source must be corex or plugin"), err)
			return
		}
	}

	matches := make([]capabilityResolveMatch, 0, 4)
	offset := 0
	var expectedTotal int64 = -1
	for {
		includeTotal := offset == 0
		views, total, listErr := h.catalog.ListCapabilities(c.Request.Context(), capservice.CapabilityListOptions{
			Source:       source,
			TenantUUID:   tenantUUID,
			Status:       []string{"published"},
			Limit:        capabilityResolvePageSize,
			Offset:       offset,
			IncludeTotal: includeTotal,
		})
		if listErr != nil {
			capability_registrydto.RespondError(c, capability_registrydto.ErrInternal, listErr)
			return
		}
		if includeTotal {
			expectedTotal = total
		}
		if len(views) == 0 {
			break
		}
		for _, view := range views {
			for _, protocol := range capability_registrydto.CapabilityViewToDTO(view, false).Protocols {
				if !strings.EqualFold(strings.TrimSpace(protocol.Channel), "rest") {
					continue
				}
				pattern := normalizeResolveEndpoint(protocol.Endpoint)
				if !strings.EqualFold(strings.TrimSpace(protocol.Method), method) {
					continue
				}
				if !routePatternMatches(pattern, endpoint) {
					continue
				}
				matches = append(matches, capabilityResolveMatch{
					CapabilityID:      strings.TrimSpace(view.Record.CapabilityID),
					PluginID:          strings.TrimSpace(view.Record.PluginID),
					Source:            capservice.CapabilitySource(view.Record),
					Protocol:          "rest",
					Method:            strings.ToUpper(strings.TrimSpace(protocol.Method)),
					PatternEndpoint:   pattern,
					RequestedEndpoint: endpoint,
					Title:             strings.TrimSpace(view.Record.Title),
				})
			}
		}
		offset += len(views)
		if expectedTotal >= 0 && int64(offset) >= expectedTotal {
			break
		}
	}

	if len(matches) == 0 {
		capability_registrydto.RespondError(c, capability_registrydto.ErrNotFound.WithHint("capability not found for method+endpoint"), nil)
		return
	}
	sort.SliceStable(matches, func(i, j int) bool {
		wi := routeWildcardCount(matches[i].PatternEndpoint)
		wj := routeWildcardCount(matches[j].PatternEndpoint)
		if wi != wj {
			return wi < wj
		}
		if matches[i].CapabilityID != matches[j].CapabilityID {
			return matches[i].CapabilityID < matches[j].CapabilityID
		}
		return matches[i].PatternEndpoint < matches[j].PatternEndpoint
	})

	dto.ResponseSuccess(c, gin.H{
		"method":            method,
		"endpoint":          endpoint,
		"source":            source,
		"primary_match":     matches[0],
		"matched_count":     len(matches),
		"candidate_matches": matches,
	})
}

func (h *tenantHandler) InvokeCapability(c *gin.Context) {
	if h == nil || h.selector == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
		return
	}
	var req capabilityInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest, err)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		respondTenantIdentityError(c, err)
		return
	}
	if strings.TrimSpace(req.CapabilityID) == "" && strings.TrimSpace(req.Intent) == "" {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("capability_id or intent is required"), nil)
		return
	}
	if strings.EqualFold(strings.TrimSpace(req.CapabilityID), "com.corex.rest.admin.gin.get_api_v1_admin_iam_members") {
		h.invokeIAMMembersCapability(c, req, tenantUUID)
		return
	}

	if strings.EqualFold(strings.TrimSpace(req.PreferredProtocol), "skill") && h.skillAdapter != nil {
		result, err := h.skillAdapter.InvokeUnified(c.Request.Context(), skillservice.UnifiedInvokeRequest{
			TenantUUID:        tenantUUID,
			Env:               strings.TrimSpace(reqctx.GetEnv(c.Request.Context())),
			CapabilityID:      strings.TrimSpace(req.CapabilityID),
			PreferredProtocol: req.PreferredProtocol,
			ToolGrantIDs:      normalizeToolGrantIDs(req.ToolGrantIDs),
			Context:           req.Context,
			Payload:           req.Payload,
			TraceID:           strings.TrimSpace(req.TraceID),
		})
		if err != nil {
			statusCode, envelope := skillservice.MapInvokeError(err)
			c.JSON(statusCode, envelope)
			return
		}
		dto.ResponseSuccess(c, gin.H{
			"trace_id":         result.TraceID,
			"status":           result.Status,
			"protocol_used":    result.ProtocolUsed,
			"fallback_used":    result.FallbackUsed,
			"result":           result.Result,
			"skill_id":         result.SkillID,
			"version":          result.Version,
			"skill_candidates": result.SkillCandidates,
		})
		return
	}

	payload := req.Payload
	if payload == nil {
		payload = map[string]interface{}{}
	}
	injectDefaultHeaders(payload, c)
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
		capability_registrydto.RespondError(c, template, err)
		return
	}

	resp := capabilityInvokeResponse{
		Payload:      result.Result,
		TraceID:      result.TraceID,
		Status:       result.Status,
		ProtocolUsed: result.ProtocolUsed,
		FallbackUsed: result.FallbackUsed,
		Result:       result.Result,
	}
	dto.ResponseSuccess(c, resp)
}

func normalizeResolveEndpoint(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	for strings.Contains(v, "//") {
		v = strings.ReplaceAll(v, "//", "/")
	}
	if len(v) > 1 && strings.HasSuffix(v, "/") {
		v = strings.TrimSuffix(v, "/")
	}
	return v
}

func routePatternMatches(pattern, actual string) bool {
	if pattern == "" || actual == "" {
		return false
	}
	patternSegs := strings.Split(strings.Trim(pattern, "/"), "/")
	actualSegs := strings.Split(strings.Trim(actual, "/"), "/")
	if len(patternSegs) != len(actualSegs) {
		return false
	}
	for i := range patternSegs {
		p := strings.TrimSpace(patternSegs[i])
		a := strings.TrimSpace(actualSegs[i])
		if p == "" || a == "" {
			return false
		}
		if isRouteParam(p) {
			continue
		}
		if !strings.EqualFold(p, a) {
			return false
		}
	}
	return true
}

func isRouteParam(seg string) bool {
	seg = strings.TrimSpace(seg)
	return strings.HasPrefix(seg, ":") || (strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}"))
}

func routeWildcardCount(pattern string) int {
	if pattern == "" {
		return 99
	}
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	count := 0
	for _, part := range parts {
		if isRouteParam(part) {
			count++
		}
	}
	return count
}

func (h *tenantHandler) GetInvocation(c *gin.Context) {
	if h == nil || h.invoker == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
		return
	}
	traceID := strings.TrimSpace(c.Param("traceId"))
	if traceID == "" {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("trace_id is required"), nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		respondTenantIdentityError(c, err)
		return
	}
	record, err := h.invoker.GetTrace(c.Request.Context(), traceID)
	if err != nil {
		template := capability_registrydto.ErrInternal
		if errors.Is(err, repo.ErrInvocationTraceNotFound) || errors.Is(err, repo.ErrCapabilityRecordNotFound) {
			template = capability_registrydto.ErrNotFound
		}
		capability_registrydto.RespondError(c, template, err)
		return
	}
	if !strings.EqualFold(record.TenantUUID, tenantUUID) {
		capability_registrydto.RespondError(c, capability_registrydto.ErrNotFound, nil)
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

func (h *tenantHandler) InvokeCapabilityStream(c *gin.Context) {
	if h == nil || h.httpClient == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable, nil)
		return
	}
	var req capabilityInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest, err)
		return
	}
	if strings.TrimSpace(req.CapabilityID) == "" {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("capability_id is required"), nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		respondTenantIdentityError(c, err)
		return
	}
	_ = tenantUUID

	restPayload, err := buildStreamRESTPayload(req.Payload)
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint(err.Error()), err)
		return
	}
	targetURL, err := resolveStreamTargetURL(c, restPayload.Endpoint)
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("invalid endpoint"), err)
		return
	}
	if len(restPayload.Query) > 0 {
		values := targetURL.Query()
		for k, v := range restPayload.Query {
			values.Set(k, v)
		}
		targetURL.RawQuery = values.Encode()
	}

	outReq, err := http.NewRequestWithContext(c.Request.Context(), restPayload.Method, targetURL.String(), bytes.NewReader(restPayload.Body))
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInternal, err)
		return
	}
	for k, v := range restPayload.Headers {
		if strings.TrimSpace(v) == "" {
			continue
		}
		outReq.Header.Set(k, v)
	}
	if outReq.Header.Get("Authorization") == "" {
		outReq.Header.Set("Authorization", c.GetHeader("Authorization"))
	}
	if outReq.Header.Get("Content-Type") == "" && len(restPayload.Body) > 0 {
		outReq.Header.Set("Content-Type", "application/json")
	}
	if outReq.Header.Get("Accept") == "" {
		outReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := h.httpClient.Do(outReq)
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInternal.WithHint("upstream request failed"), err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		c.JSON(resp.StatusCode, gin.H{
			"code":    "upstream_error",
			"message": strings.TrimSpace(string(raw)),
		})
		return
	}
	if ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type"))); !strings.Contains(ct, "text/event-stream") {
		raw, _ := io.ReadAll(resp.Body)
		c.JSON(http.StatusBadGateway, gin.H{
			"code":    "upstream_not_sse",
			"message": "upstream response is not text/event-stream",
			"detail":  string(raw),
		})
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := c.Writer.Write(buf[:n]); wErr != nil {
				return
			}
			if f, ok := c.Writer.(http.Flusher); ok {
				f.Flush()
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return
			}
			return
		}
	}
}

type streamRESTPayload struct {
	Method   string
	Endpoint string
	Headers  map[string]string
	Query    map[string]string
	Body     []byte
}

func buildStreamRESTPayload(payload map[string]interface{}) (streamRESTPayload, error) {
	if payload == nil {
		return streamRESTPayload{}, errors.New("payload is required")
	}
	method := strings.ToUpper(strings.TrimSpace(getString(payload["method"])))
	if method == "" {
		method = http.MethodPost
	}
	endpoint := strings.TrimSpace(getString(payload["endpoint"]))
	if endpoint == "" {
		return streamRESTPayload{}, errors.New("payload.endpoint is required")
	}
	headers := mapStringString(payload["headers"])
	query := mapStringString(payload["query"])

	var body []byte
	if b, ok := payload["body"]; ok && b != nil {
		raw, err := json.Marshal(b)
		if err != nil {
			return streamRESTPayload{}, err
		}
		body = raw
	}
	return streamRESTPayload{
		Method:   method,
		Endpoint: endpoint,
		Headers:  headers,
		Query:    query,
		Body:     body,
	}, nil
}

func resolveStreamTargetURL(c *gin.Context, endpoint string) (*url.URL, error) {
	if strings.HasPrefix(strings.ToLower(endpoint), "http://") || strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		return url.Parse(endpoint)
	}
	scheme := "http"
	if c != nil && c.Request != nil && c.Request.TLS != nil {
		scheme = "https"
	} else if c != nil && strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := ""
	if c != nil && c.Request != nil {
		host = c.Request.Host
	}
	if strings.TrimSpace(host) == "" {
		return nil, errors.New("request host is empty")
	}
	return url.Parse(scheme + "://" + host + "/" + strings.TrimLeft(endpoint, "/"))
}

func mapStringString(v interface{}) map[string]string {
	result := make(map[string]string)
	switch typed := v.(type) {
	case map[string]interface{}:
		for key, value := range typed {
			result[key] = strings.TrimSpace(getString(value))
		}
	case map[string]string:
		for key, value := range typed {
			result[key] = strings.TrimSpace(value)
		}
	}
	return result
}

func getString(v interface{}) string {
	switch typed := v.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
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
	Payload      map[string]interface{} `json:"payload,omitempty"`
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
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		return "", reqctx.ErrTenantUUIDMissing
	}
	return reqctx.CanonicalTenantUUID(tenant)
}

func respondTenantIdentityError(c *gin.Context, err error) {
	if errors.Is(err, reqctx.ErrTenantUUIDMissing) {
		capability_registrydto.RespondError(c, capability_registrydto.ErrTenantUUIDMissing, err)
		return
	}
	capability_registrydto.RespondError(c, capability_registrydto.ErrTenantUUIDInvalid, err)
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

func selectInvokeErrorTemplate(err error) capability_registrydto.ErrorTemplate {
	switch {
	case errors.Is(err, capservice.ErrManualUpgradeRequired):
		return capability_registrydto.ErrVersionLocked
	case errors.Is(err, capservice.ErrSelectorCapabilityRequired):
		return capability_registrydto.ErrNotFound.WithHint("capability not found or not published for tenant")
	case errors.Is(err, capservice.ErrSelectorCapabilityForbidden):
		return capability_registrydto.ErrCapabilityForbidden
	case errors.Is(err, capservice.ErrSelectorTenantRequired):
		return capability_registrydto.ErrTenantUUIDMissing
	case errors.Is(err, capservice.ErrSelectorSafeModeActive):
		return capability_registrydto.ErrSafeModeActive
	case errors.Is(err, capservice.ErrSelectorToolGrantRequired):
		return capability_registrydto.ErrToolGrantMissing
	case errors.Is(err, capservice.ErrSelectorFeatureFlagMissing):
		return capability_registrydto.ErrFeatureFlagMissing
	case errors.Is(err, capservice.ErrSelectorUnavailable):
		return capability_registrydto.ErrUnavailable
	default:
		return capability_registrydto.ErrInvokeFailed
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
func injectDefaultHeaders(payload map[string]interface{}, c *gin.Context) {
	if payload == nil || c == nil {
		return
	}
	headers, ok := payload["headers"].(map[string]interface{})
	if !ok || headers == nil {
		headers = make(map[string]interface{})
	}
	if auth := strings.TrimSpace(c.GetHeader("Authorization")); auth != "" {
		if _, exists := headers["Authorization"]; !exists {
			headers["Authorization"] = auth
		}
	}
	if traceID := strings.TrimSpace(c.GetHeader("X-Trace-Id")); traceID != "" {
		if _, exists := headers["X-Trace-Id"]; !exists {
			headers["X-Trace-Id"] = traceID
		}
	}
	payload["headers"] = headers
}

func (h *tenantHandler) invokeIAMMembersCapability(c *gin.Context, req capabilityInvokeRequest, tenantUUID string) {
	if h == nil || h.memberSvc == nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrUnavailable.WithHint("iam member service unavailable"), nil)
		return
	}
	query := mapStringString(req.Payload["query"])
	page := parsePositiveInt(query["page"], 1)
	pageSize := parsePositiveInt(query["page_size"], 100)
	if pageSize > 100 {
		pageSize = 100
	}
	var status *int16
	if raw := strings.TrimSpace(query["status"]); raw != "" && !strings.EqualFold(raw, "all") {
		parsed, err := strconv.ParseInt(raw, 10, 16)
		if err != nil {
			capability_registrydto.RespondError(c, capability_registrydto.ErrInvalidRequest.WithHint("query.status must be int16"), err)
			return
		}
		v := int16(parsed)
		status = &v
	}
	rows, total, err := h.memberSvc.ListMembersByTenantUUID(c.Request.Context(), tenantUUID, iamsvc.ListMembersOption{
		Page:      page,
		PageSize:  pageSize,
		Keyword:   strings.TrimSpace(query["q"]),
		Status:    status,
		Recursive: true,
	})
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInternal, err)
		return
	}
	logger.InfoF(c.Request.Context(), "[tenant-invocations] capability=%s protocol=internal tenant_uuid=%s page=%d page_size=%d total=%d",
		strings.TrimSpace(req.CapabilityID),
		tenantUUID,
		page,
		pageSize,
		total,
	)
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		item := gin.H{
			"Member": row.Member,
			"User":   row.User,
		}
		if len(row.DeptIDs) > 0 {
			item["DeptIDs"] = row.DeptIDs
		}
		items = append(items, item)
	}
	dto.ResponseSuccess(c, gin.H{
		"trace_id":      strings.TrimSpace(req.TraceID),
		"status":        "completed",
		"protocol_used": "internal",
		"fallback_used": false,
		"result": gin.H{
			"items": items,
			"pagination": gin.H{
				"total":     total,
				"page":      page,
				"page_size": pageSize,
			},
		},
	})
}
