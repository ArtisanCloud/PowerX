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
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegisterAPIRoutes 注册集成网关管理端接口。
func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil || deps.IntegrationGateway == nil || deps.IntegrationGateway.Manager == nil {
		return
	}

	handler := &AdminHandler{svc: deps.IntegrationGateway.Manager}

	group := protected.Group("/admin/integration")
	group.POST("/routes", handler.CreateRoute)
	group.GET("/routes", handler.ListRoutes)
	group.GET("/routes/:route_id", handler.GetRoute)
	group.PATCH("/routes/:route_id", handler.UpdateRoute)
	group.POST("/routes/:route_id:suspend", handler.SuspendRoute)
	group.POST("/routes/:route_id:resume", handler.ResumeRoute)
	group.POST("/routes/:route_id:retire", handler.RetireRoute)
	group.GET("/routes/:route_id/versions", handler.ListVersions)
}

// AdminHandler 负责管理端 HTTP 请求。
type AdminHandler struct {
	svc *manager.Service
}

type createRouteRequest struct {
	TenantID     string                   `json:"tenant_id" binding:"required"`
	RouteSlug    string                   `json:"route_slug" binding:"required"`
	CapabilityID string                   `json:"capability_id" binding:"required"`
	ToolGrantIDs []string                 `json:"tool_grant_ids"`
	Channels     []string                 `json:"channels"`
	RateLimit    *manager.RateLimitPolicy `json:"rate_limit"`
	EventTopics  *manager.EventTopics     `json:"event_topics"`
	Description  string                   `json:"description"`
}

type updateRouteRequest struct {
	TenantID     string                   `json:"tenant_id" binding:"required"`
	CapabilityID string                   `json:"capability_id"`
	ToolGrantIDs []string                 `json:"tool_grant_ids"`
	Channels     []string                 `json:"channels"`
	RateLimit    *manager.RateLimitPolicy `json:"rate_limit"`
	EventTopics  *manager.EventTopics     `json:"event_topics"`
	Description  *string                  `json:"description"`
	Status       *string                  `json:"status"`
}

type lifecycleRequest struct {
	TenantID string `json:"tenant_id" binding:"required"`
	Reason   string `json:"reason"`
}

type routeResponse struct {
	RouteID         string                  `json:"route_id"`
	TenantID        string                  `json:"tenant_id"`
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
		TenantID:       route.TenantID,
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

	route, err := h.svc.CreateRoute(c.Request.Context(), manager.CreateRouteInput{
		TenantID:     strings.TrimSpace(req.TenantID),
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
	tenantID := strings.TrimSpace(c.Query("tenant_id"))
	if tenantID == "" {
		dto.RespondErrorFrom(c, dto.NewBadRequest("tenant_id is required", nil))
		return
	}
	capabilityID := strings.TrimSpace(c.Query("capability_id"))
	lifecycle := strings.TrimSpace(c.Query("lifecycle_state"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	routes, total, err := h.svc.ListRoutes(c.Request.Context(), manager.ListRoutesInput{
		TenantID:       tenantID,
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

	route, err := h.svc.UpdateRoute(c.Request.Context(), manager.UpdateRouteInput{
		RouteID:      routeID,
		TenantID:     strings.TrimSpace(req.TenantID),
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
	if strings.TrimSpace(req.TenantID) == "" {
		dto.RespondErrorFrom(c, dto.NewBadRequest("tenant_id is required", nil))
		return
	}

	route, err := h.svc.ChangeLifecycle(c.Request.Context(), manager.ChangeLifecycleInput{
		RouteID:  routeID,
		TenantID: strings.TrimSpace(req.TenantID),
		Actor:    actorFromHeader(c),
		Action:   action,
		Reason:   req.Reason,
		Version:  version,
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
