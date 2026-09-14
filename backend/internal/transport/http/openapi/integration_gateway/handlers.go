package integration_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	capaccess "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	integrationTenant "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type tenantHandler struct {
	svc tenantService
}

// The handler depends on operations, allowing the wire contract to be tested
// independently of routing and storage dependency failures.
type tenantService interface {
	ListRoutes(context.Context, string, string, string) ([]manager.Route, error)
	GetRoute(context.Context, string, string) (manager.Route, error)
	Invoke(context.Context, integrationTenant.InvokeInput) (integrationTenant.InvokeResult, error)
}

var _ tenantService = (*integrationTenant.Service)(nil)

type routeSummaryResponse struct {
	RouteID        string   `json:"route_id"`
	RouteSlug      string   `json:"route_slug"`
	CapabilityID   string   `json:"capability_id"`
	Channels       []string `json:"channels"`
	LifecycleState string   `json:"lifecycle_state"`
	Status         string   `json:"status"`
	UpdatedAt      string   `json:"updated_at"`
}

type routeDetailResponse struct {
	RouteID        string                  `json:"route_id"`
	RouteSlug      string                  `json:"route_slug"`
	CapabilityID   string                  `json:"capability_id"`
	ToolGrantIDs   []string                `json:"tool_grant_ids"`
	Channels       []string                `json:"channels"`
	RateLimit      manager.RateLimitPolicy `json:"rate_limit"`
	LifecycleState string                  `json:"lifecycle_state"`
	Status         string                  `json:"status"`
	Description    string                  `json:"description,omitempty"`
	CurrentVersion uint32                  `json:"current_version"`
	CreatedAt      string                  `json:"created_at"`
	UpdatedAt      string                  `json:"updated_at"`
}

type invokeRequest struct {
	Payload        map[string]any `json:"payload" binding:"required"`
	IdempotencyKey string         `json:"idempotency_key"`
	Context        map[string]any `json:"context"`
}

type invokeResponse struct {
	Result             map[string]any `json:"result,omitempty"`
	RoutedCapabilityID string         `json:"routed_capability_id"`
	RoutedAdapter      string         `json:"routed_adapter,omitempty"`
	TraceID            string         `json:"trace_id"`
	DispatchedAt       string         `json:"dispatched_at"`
}

func (h *tenantHandler) ListRoutes(c *gin.Context) {
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		respondTenantIdentityError(c, err)
		return
	}

	capabilityID := strings.TrimSpace(c.Query("capability_id"))
	channel := strings.TrimSpace(c.Query("channel"))

	routes, err := h.svc.ListRoutes(c.Request.Context(), tenantUUID, capabilityID, channel)
	if err != nil {
		respondTenantError(c, err)
		return
	}

	items := make([]routeSummaryResponse, 0, len(routes))
	for _, route := range routes {
		items = append(items, routeSummaryResponse{
			RouteID:        route.RouteID.String(),
			RouteSlug:      route.RouteSlug,
			CapabilityID:   route.CapabilityID,
			Channels:       route.Channels,
			LifecycleState: route.LifecycleState,
			Status:         route.Status,
			UpdatedAt:      route.UpdatedAt.Format(time.RFC3339),
		})
	}

	dto.ResponseList(c, items, nil)
}

func (h *tenantHandler) GetRoute(c *gin.Context) {
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		respondTenantIdentityError(c, err)
		return
	}

	routeSlug := c.Param("route_slug")
	route, err := h.svc.GetRoute(c.Request.Context(), tenantUUID, routeSlug)
	if err != nil {
		respondTenantError(c, err)
		return
	}

	resp := routeDetailResponse{
		RouteID:        route.RouteID.String(),
		RouteSlug:      route.RouteSlug,
		CapabilityID:   route.CapabilityID,
		ToolGrantIDs:   route.ToolGrantIDs,
		Channels:       route.Channels,
		RateLimit:      route.RateLimit,
		LifecycleState: route.LifecycleState,
		Status:         route.Status,
		Description:    route.Description,
		CurrentVersion: route.CurrentVersion,
		CreatedAt:      route.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      route.UpdatedAt.Format(time.RFC3339),
	}

	dto.ResponseSuccess(c, resp)
}

func (h *tenantHandler) InvokeRoute(c *gin.Context) {
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		respondTenantIdentityError(c, err)
		return
	}

	var req invokeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20))
	if err := decodeInvokeBody(decoder, &req); err != nil {
		dto.RespondErrorFrom(c, gatewayError(c, 400, "CAPABILITY_INVALID_ARGUMENT"))
		return
	}

	input := integrationTenant.InvokeInput{
		TenantUUID:     tenantUUID,
		RouteSlug:      c.Param("route_slug"),
		Channel:        "http",
		Payload:        req.Payload,
		Context:        req.Context,
		IdempotencyKey: req.IdempotencyKey,
		TraceID:        c.GetHeader("X-Trace-Id"),
	}

	result, err := h.svc.Invoke(c.Request.Context(), input)
	if err != nil {
		var rlErr integrationTenant.RateLimitError
		if errors.As(err, &rlErr) {
			if result.TraceID != "" {
				c.Header("X-Trace-Id", result.TraceID)
			}
			details := map[string]interface{}{
				"retry_after": rlErr.RetryAfter.String(),
				"quota_scope": rlErr.Scope,
			}
			dto.RespondErrorFrom(c, dto.WithDetails(gatewayError(c, http.StatusTooManyRequests, "CAPABILITY_RATE_LIMITED"), details))
			return
		}
		if result.TraceID != "" {
			c.Header("X-Trace-Id", result.TraceID)
		}
		respondTenantError(c, err)
		return
	}

	if result.TraceID != "" {
		c.Header("X-Trace-Id", result.TraceID)
	}

	resp := invokeResponse{
		Result:             result.Result,
		RoutedCapabilityID: result.RoutedCapabilityID,
		RoutedAdapter:      result.RoutedAdapter,
		TraceID:            result.TraceID,
	}
	if !result.DispatchedAt.IsZero() {
		resp.DispatchedAt = result.DispatchedAt.Format(time.RFC3339)
	}

	switch result.Status {
	case integrationTenant.InvokeStatusDenied:
		dto.RespondErrorFrom(c, gatewayError(c, http.StatusForbidden, "CAPABILITY_FORBIDDEN"))
		return
	case integrationTenant.InvokeStatusFailed:
		dto.RespondErrorFrom(c, gatewayError(c, http.StatusServiceUnavailable, "CAPABILITY_UPSTREAM_DEPENDENCY"))
		return
	case integrationTenant.InvokeStatusAccepted:
		dto.ResponseSuccessWithStatus(c, http.StatusAccepted, resp)
	case integrationTenant.InvokeStatusOK:
		dto.ResponseSuccess(c, resp)
	default:
		dto.RespondErrorFrom(c, gatewayError(c, http.StatusServiceUnavailable, "CAPABILITY_UPSTREAM_DEPENDENCY"))
	}
}

func decodeInvokeBody(d *json.Decoder, req *invokeRequest) error {
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("capability.invalid_body")
	}
	fields := make(map[string]json.RawMessage)
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || (key != "payload" && key != "context" && key != "idempotency_key") || fields[key] != nil {
			return errors.New("capability.invalid_field")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return err
		}
		if string(value) == "null" {
			return errors.New("capability.invalid_field")
		}
		fields[key] = value
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return errors.New("capability.invalid_body")
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(encoded, req); err != nil {
		return err
	}
	if req.Payload == nil {
		return errors.New("capability.payload_required")
	}
	return nil
}

func tenantUUIDFromRequest(c *gin.Context) (string, error) {
	if c == nil {
		return "", reqctx.ErrTenantUUIDMissing
	}
	for key := range c.Request.URL.Query() {
		if strings.Contains(strings.ToLower(key), "tenant") {
			return "", errors.New("capability.tenant_override")
		}
	}
	for key := range c.Request.Header {
		if strings.Contains(strings.ToLower(key), "tenant") {
			return "", errors.New("capability.tenant_override")
		}
	}
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenantUUID == "" {
		return "", reqctx.ErrTenantUUIDMissing
	}
	return reqctx.CanonicalTenantUUID(tenantUUID)
}

func respondTenantIdentityError(c *gin.Context, err error) {
	if errors.Is(err, reqctx.ErrTenantUUIDMissing) {
		dto.RespondErrorFrom(c, gatewayError(c, http.StatusUnauthorized, "CAPABILITY_UNAUTHORIZED"))
		return
	}
	dto.RespondErrorFrom(c, gatewayError(c, http.StatusBadRequest, "CAPABILITY_INVALID_ARGUMENT"))
}

func respondTenantError(c *gin.Context, err error) {
	var accessErr *capaccess.DirectGrantError
	if errors.As(err, &accessErr) {
		dto.RespondErrorFrom(c, gatewayError(c, accessErr.Status, accessErr.Error()))
		return
	}
	var routeErr integrationTenant.ErrRouteNotAccessible
	if errors.As(err, &routeErr) {
		dto.RespondErrorFrom(c, gatewayError(c, 404, "CAPABILITY_ROUTE_NOT_FOUND"))
		return
	}
	var channelErr integrationTenant.ErrChannelDisabled
	if errors.As(err, &channelErr) {
		dto.RespondErrorFrom(c, gatewayError(c, 404, "CAPABILITY_ROUTE_NOT_FOUND"))
		return
	}
	var grantErr integrationTenant.ErrToolGrantDenied
	if errors.As(err, &grantErr) {
		dto.RespondErrorFrom(c, gatewayError(c, 403, "CAPABILITY_FORBIDDEN"))
		return
	}
	dto.RespondErrorFrom(c, gatewayError(c, 503, "CAPABILITY_UPSTREAM_DEPENDENCY"))
}
