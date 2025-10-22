package integration_gateway

import (
	"errors"
	"net/http"
	"strings"
	"time"

	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	integrationTenant "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type tenantHandler struct {
	svc *integrationTenant.Service
}

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
	tenantID := resolveTenantID(c)
	if tenantID == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "missing tenant identifier", nil)
		return
	}

	capabilityID := strings.TrimSpace(c.Query("capability_id"))
	channel := strings.TrimSpace(c.Query("channel"))

	routes, err := h.svc.ListRoutes(c.Request.Context(), tenantID, capabilityID, channel)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "list routes failed", err)
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
	tenantID := resolveTenantID(c)
	if tenantID == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "missing tenant identifier", nil)
		return
	}

	routeSlug := c.Param("route_slug")
	route, err := h.svc.GetRoute(c.Request.Context(), tenantID, routeSlug)
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
	tenantID := resolveTenantID(c)
	if tenantID == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "missing tenant identifier", nil)
		return
	}

	var req invokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	input := integrationTenant.InvokeInput{
		TenantID:       tenantID,
		RouteSlug:      c.Param("route_slug"),
		Channel:        "http",
		Payload:        req.Payload,
		Context:        req.Context,
		IdempotencyKey: req.IdempotencyKey,
		TraceID:        c.GetHeader("X-Trace-Id"),
		Actor:          c.GetHeader("Authorization"),
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
			dto.ResponseErrorWithDetails(c, http.StatusTooManyRequests, "rate limit exceeded", err, details)
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
		dto.ResponseError(c, http.StatusForbidden, "tool grant denied", nil)
		return
	case integrationTenant.InvokeStatusFailed:
		errMsg := result.ErrorMessage
		if errMsg == "" {
			errMsg = "capability invocation failed"
		}
		dto.ResponseError(c, http.StatusFailedDependency, errMsg, nil)
		return
	case integrationTenant.InvokeStatusAccepted:
		dto.ResponseSuccessWithStatus(c, http.StatusAccepted, resp)
	default:
		dto.ResponseSuccess(c, resp)
	}
}

func resolveTenantID(c *gin.Context) string {
	if tenant := strings.TrimSpace(c.GetHeader("X-PowerX-Tenant")); tenant != "" {
		return tenant
	}
	if tenant := strings.TrimSpace(c.Query("tenant_id")); tenant != "" {
		return tenant
	}
	return ""
}

func respondTenantError(c *gin.Context, err error) {
	var routeErr integrationTenant.ErrRouteNotAccessible
	if errors.As(err, &routeErr) {
		dto.ResponseError(c, http.StatusNotFound, "route not accessible", err)
		return
	}
	var channelErr integrationTenant.ErrChannelDisabled
	if errors.As(err, &channelErr) {
		dto.ResponseError(c, http.StatusNotFound, "channel disabled", err)
		return
	}
	var grantErr integrationTenant.ErrToolGrantDenied
	if errors.As(err, &grantErr) {
		dto.ResponseError(c, http.StatusForbidden, "tool grant denied", err)
		return
	}
	dto.ResponseError(c, http.StatusInternalServerError, "integration gateway error", err)
}
