package integration_gateway

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegisterAPIRoutes 注册集成网关管理端接口。
func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil || deps.IntegrationGateway == nil || deps.IntegrationGateway.Manager == nil || deps.DB == nil {
		return
	}

	handler := &AdminHandler{svc: deps.IntegrationGateway.Manager}

	group := protected.Group("/admin/integration")
	protected.GET("/admin/gateway/meta", handleGatewayMeta)
	group.POST("/routes", handler.CreateRoute)
	group.GET("/routes", handler.ListRoutes)
	group.GET("/routes/:route_id", handler.GetRoute)
	group.PATCH("/routes/:route_id", handler.UpdateRoute)
	group.POST("/routes/:route_id/suspend", handler.SuspendRoute)
	group.POST("/routes/:route_id/resume", handler.ResumeRoute)
	group.POST("/routes/:route_id/retire", handler.RetireRoute)
	group.GET("/routes/:route_id/versions", handler.ListVersions)

	apiKeyHandler := NewAPIKeyAdminHandler(deps.DB)
	group.POST("/api-key-profiles", apiKeyHandler.CreateAPIKeyProfile)
	group.GET("/api-key-profiles", apiKeyHandler.ListAPIKeyProfiles)
	group.PATCH("/api-key-profiles/:profile_id", apiKeyHandler.UpdateAPIKeyProfile)
	group.GET("/api-key-profiles/:profile_id/permissions", apiKeyHandler.GetAPIKeyProfilePermissions)
	group.PUT("/api-key-profiles/:profile_id/permissions", apiKeyHandler.SetAPIKeyProfilePermissions)
	group.GET("/permissions/catalog", apiKeyHandler.ListAPIKeyPermissionCatalog)
	group.POST("/api-keys", apiKeyHandler.CreateAPIKey)
	group.GET("/api-keys", apiKeyHandler.ListAPIKeys)
	group.GET("/api-keys/:key_id", apiKeyHandler.GetAPIKey)
	group.POST("/api-keys/:key_id/revoke", apiKeyHandler.RevokeAPIKey)
	group.POST("/api-keys/:key_id/rotate", apiKeyHandler.RotateAPIKey)
	group.DELETE("/api-keys/:key_id", apiKeyHandler.DeleteAPIKey)
}

func handleGatewayMeta(c *gin.Context) {
	apiPrefix := inferAPIPrefix(c)
	baseURL := requestBaseURL(c)

	dto.ResponseSuccess(c, gin.H{
		"base_url":       baseURL,
		"api_prefix":     apiPrefix,
		"http_base":      strings.TrimRight(baseURL, "/") + apiPrefix,
		"ws":             gin.H{"bus_endpoint": "/api/ws"},
		"default_scheme": "apikey",
		"auth": gin.H{
			"schemes": []gin.H{
				{
					"id":     "apikey",
					"header": "Authorization",
					"format": "ApiKey <key>",
				},
				{
					"id":     "bearer",
					"header": "Authorization",
					"format": "Bearer <token>",
				},
			},
			"single_credential_per_request": true,
			"conflict_policy":               "prefer_header_scheme",
		},
		"examples": gin.H{
			"llm_invoke_path":                    apiPrefix + "/ai/llm/invoke",
			"tenant_capability_invoke_path":      apiPrefix + "/tenant/invocations",
			"integration_route_invoke_path_tmpl": apiPrefix + "/tenant/integration/routes/{route_slug}/invoke",
			"cap_list_path":                      apiPrefix + "/admin/capabilities",
		},
	})
}

func inferAPIPrefix(c *gin.Context) string {
	const marker = "/admin/gateway/meta"
	path := strings.TrimSpace(c.Request.URL.Path)
	if idx := strings.Index(path, marker); idx >= 0 {
		prefix := strings.TrimSpace(path[:idx])
		if prefix == "" {
			return "/api/v1"
		}
		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		return strings.TrimRight(prefix, "/")
	}
	return "/api/v1"
}

func requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); forwarded != "" {
		if i := strings.Index(forwarded, ","); i >= 0 {
			forwarded = forwarded[:i]
		}
		forwarded = strings.TrimSpace(forwarded)
		if forwarded != "" {
			scheme = forwarded
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

// AdminHandler 负责管理端 HTTP 请求。
type AdminHandler struct {
	svc *manager.Service
}

type createRouteRequest struct {
	TenantUUID   string                   `json:"tenant_uuid,omitempty"`
	RouteSlug    string                   `json:"route_slug" binding:"required"`
	CapabilityID string                   `json:"capability_id" binding:"required"`
	ToolGrantIDs []string                 `json:"tool_grant_ids"`
	Channels     []string                 `json:"channels"`
	RateLimit    *manager.RateLimitPolicy `json:"rate_limit"`
	EventTopics  *manager.EventTopics     `json:"event_topics"`
	Description  string                   `json:"description"`
}

type updateRouteRequest struct {
	TenantUUID   string                   `json:"tenant_uuid,omitempty"`
	CapabilityID string                   `json:"capability_id"`
	ToolGrantIDs []string                 `json:"tool_grant_ids"`
	Channels     []string                 `json:"channels"`
	RateLimit    *manager.RateLimitPolicy `json:"rate_limit"`
	EventTopics  *manager.EventTopics     `json:"event_topics"`
	Description  *string                  `json:"description"`
	Status       *string                  `json:"status"`
}

type lifecycleRequest struct {
	TenantUUID string `json:"tenant_uuid,omitempty"`
	Reason     string `json:"reason"`
}

type routeResponse struct {
	RouteID         string                  `json:"route_id"`
	TenantUUID      string                  `json:"tenant_uuid"`
	RouteSlug       string                  `json:"route_slug"`
	CapabilityID    string                  `json:"capability_id"`
	ToolGrantIDs    []string                `json:"tool_grant_ids"`
	Channels        []string                `json:"channels"`
	RateLimit       manager.RateLimitPolicy `json:"rate_limit"`
	EventTopics     manager.EventTopics     `json:"event_topics"`
	LifecycleState  string                  `json:"lifecycle_state"`
	Status          string                  `json:"status"`
	CurrentVersion  uint32                  `json:"current_version"`
	Description     string                  `json:"description,omitempty"`
	CreatedAt       string                  `json:"created_at"`
	UpdatedAt       string                  `json:"updated_at"`
	LastActivityAt  *string                 `json:"last_activity_at,omitempty"`
	LastPublishedAt *string                 `json:"last_published_at,omitempty"`
}

func routeToResponse(route manager.Route) routeResponse {
	resp := routeResponse{
		RouteID:        route.RouteID.String(),
		TenantUUID:     route.TenantUUID,
		RouteSlug:      route.RouteSlug,
		CapabilityID:   route.CapabilityID,
		ToolGrantIDs:   route.ToolGrantIDs,
		Channels:       route.Channels,
		RateLimit:      route.RateLimit,
		EventTopics:    route.EventTopics,
		LifecycleState: route.LifecycleState,
		Status:         route.Status,
		CurrentVersion: route.CurrentVersion,
		Description:    route.Description,
		CreatedAt:      route.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      route.UpdatedAt.Format(time.RFC3339),
	}
	if route.LastActivityAt != nil {
		v := route.LastActivityAt.Format(time.RFC3339)
		resp.LastActivityAt = &v
	}
	if route.LastPublishedAt != nil {
		v := route.LastPublishedAt.Format(time.RFC3339)
		resp.LastPublishedAt = &v
	}
	return resp
}

func (h *AdminHandler) CreateRoute(c *gin.Context) {
	var req createRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	canonical, err := reqctx.RequireTenantUUID(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	route, err := h.svc.CreateRoute(c.Request.Context(), manager.CreateRouteInput{
		TenantUUID:   canonical,
		Actor:        actorFromHeader(c),
		RouteSlug:    req.RouteSlug,
		CapabilityID: req.CapabilityID,
		ToolGrantIDs: req.ToolGrantIDs,
		Channels:     req.Channels,
		RateLimit:    req.RateLimit,
		EventTopics:  req.EventTopics,
		Description:  req.Description,
	})
	if err != nil {
		dto.RespondErrorFrom(c, mapError(err))
		return
	}

	setETag(c, route.CurrentVersion)
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, routeToResponse(route))
}

func (h *AdminHandler) ListRoutes(c *gin.Context) {
	canonical, err := reqctx.RequireTenantUUID(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}
	capabilityID := strings.TrimSpace(c.Query("capability_id"))
	lifecycle := strings.TrimSpace(c.Query("lifecycle_state"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	routes, total, err := h.svc.ListRoutes(c.Request.Context(), manager.ListRoutesInput{
		TenantUUID:     canonical,
		CapabilityID:   capabilityID,
		LifecycleState: lifecycle,
		Page:           page,
		PageSize:       pageSize,
	})
	if err != nil {
		dto.RespondErrorFrom(c, mapError(err))
		return
	}

	items := make([]routeResponse, 0, len(routes))
	for _, rt := range routes {
		items = append(items, routeToResponse(rt))
	}

	dto.ResponseSuccess(c, dto.ListResponse{
		Items: items,
		Pagination: &dto.PaginationResponse{
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
	})
}

func (h *AdminHandler) GetRoute(c *gin.Context) {
	routeID, err := uuid.Parse(c.Param("route_id"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid route_id", err))
		return
	}

	route, err := h.svc.GetRoute(c.Request.Context(), routeID)
	if err != nil {
		dto.RespondErrorFrom(c, mapError(err))
		return
	}

	setETag(c, route.CurrentVersion)
	dto.ResponseSuccess(c, routeToResponse(route))
}

func (h *AdminHandler) UpdateRoute(c *gin.Context) {
	routeID, err := uuid.Parse(c.Param("route_id"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid route_id", err))
		return
	}

	version, err := parseIfMatch(c.GetHeader("If-Match"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewError(http.StatusPreconditionFailed, "If-Match header required", err))
		return
	}

	var req updateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	canonical, err := reqctx.RequireTenantUUID(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	route, err := h.svc.UpdateRoute(c.Request.Context(), manager.UpdateRouteInput{
		RouteID:      routeID,
		TenantUUID:   canonical,
		Actor:        actorFromHeader(c),
		Version:      version,
		CapabilityID: req.CapabilityID,
		ToolGrantIDs: req.ToolGrantIDs,
		Channels:     req.Channels,
		RateLimit:    req.RateLimit,
		EventTopics:  req.EventTopics,
		Description:  req.Description,
		Status:       req.Status,
	})
	if err != nil {
		dto.RespondErrorFrom(c, mapError(err))
		return
	}

	setETag(c, route.CurrentVersion)
	dto.ResponseSuccess(c, routeToResponse(route))
}

func (h *AdminHandler) SuspendRoute(c *gin.Context) { h.lifecycle(c, "suspend") }
func (h *AdminHandler) ResumeRoute(c *gin.Context)  { h.lifecycle(c, "resume") }
func (h *AdminHandler) RetireRoute(c *gin.Context)  { h.lifecycle(c, "retire") }

func (h *AdminHandler) lifecycle(c *gin.Context, action string) {
	routeID, err := uuid.Parse(c.Param("route_id"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid route_id", err))
		return
	}
	version, err := parseIfMatchOptional(c.GetHeader("If-Match"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewError(http.StatusPreconditionFailed, "invalid If-Match header", err))
		return
	}

	var req lifecycleRequest
	err = c.ShouldBindJSON(&req)
	if err != nil && !errors.Is(err, io.EOF) {
		dto.ResponseValidationError(c, err)
		return
	}
	canonical, err := reqctx.RequireTenantUUID(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	route, err := h.svc.ChangeLifecycle(c.Request.Context(), manager.ChangeLifecycleInput{
		RouteID:    routeID,
		TenantUUID: canonical,
		Actor:      actorFromHeader(c),
		Action:     action,
		Reason:     req.Reason,
		Version:    version,
	})
	if err != nil {
		dto.RespondErrorFrom(c, mapError(err))
		return
	}

	setETag(c, route.CurrentVersion)
	dto.ResponseSuccess(c, routeToResponse(route))
}

func (h *AdminHandler) ListVersions(c *gin.Context) {
	routeID, err := uuid.Parse(c.Param("route_id"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid route_id", err))
		return
	}

	versions, err := h.svc.ListVersions(c.Request.Context(), routeID)
	if err != nil {
		dto.RespondErrorFrom(c, mapError(err))
		return
	}

	dto.ResponseSuccess(c, gin.H{"items": versions})
}

func actorFromHeader(c *gin.Context) string {
	if v := c.GetHeader("X-Actor-ID"); v != "" {
		return v
	}
	if v := c.GetHeader("X-User-Id"); v != "" {
		return v
	}
	return "admin"
}

func parseIfMatch(value string) (uint32, error) {
	if value == "" {
		return 0, errors.New("If-Match required")
	}
	return parseIfMatchOptional(value)
}

func parseIfMatchOptional(value string) (uint32, error) {
	if value == "" {
		return 0, nil
	}
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "W/")
	trimmed = strings.Trim(trimmed, "\"")
	v, err := strconv.Atoi(trimmed)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("invalid If-Match value: %s", value)
	}
	return uint32(v), nil
}

func setETag(c *gin.Context, version uint32) {
	c.Header("ETag", fmt.Sprintf("W/\"%d\"", version))
}

func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, manager.ErrSlugConflict):
		return dto.NewConflict("route slug already exists", err)
	case errors.Is(err, manager.ErrVersionConflict):
		return dto.NewError(http.StatusPreconditionFailed, "version conflict", err)
	case errors.Is(err, manager.ErrRouteNotFound):
		return dto.NewNotFound("route not found", err)
	default:
		return dto.NewInternal("integration gateway operation failed", err)
	}
}
