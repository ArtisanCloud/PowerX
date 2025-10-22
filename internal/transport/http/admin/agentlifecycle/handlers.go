package agentlifecycle

import (
	"errors"
	"fmt"
	nethttp "net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler 提供生命周期相关的 HTTP 入口。
type Handler struct {
	service *agent_lifecycle.Service
}

// NewHandler 构造 Handler。
func NewHandler(deps *shared.Deps) *Handler {
	if deps == nil || deps.AgentLifecycle == nil || deps.AgentLifecycle.Service == nil {
		return &Handler{}
	}
	return &Handler{service: deps.AgentLifecycle.Service}
}

// RegisterAgent 处理注册请求。
func (h *Handler) RegisterAgent(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, nethttp.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}

	var req registerAgentRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	result, err := h.service.Register(c.Request.Context(), toRegisterInput(req))
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, nethttp.StatusCreated, fromAgent(result))
}

// GetAgent 返回单个代理档案。
func (h *Handler) GetAgent(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, nethttp.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}

	agentID, err := uuid.Parse(c.Param("agent_id"))
	if err != nil {
		dto.ResponseError(c, nethttp.StatusBadRequest, "invalid agent_id", err)
		return
	}

	agent, err := h.service.Get(c.Request.Context(), agentID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, fromAgent(agent))
}

// ListAgents 返回租户代理列表。
func (h *Handler) ListAgents(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, nethttp.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}

	tenantID := c.Query("tenant_id")
	if tenantID == "" {
		dto.ResponseValidationError(c, fmt.Errorf("tenant_id is required"))
		return
	}

	agents, err := h.service.ListByTenant(c.Request.Context(), tenantID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	views := make([]agentResponse, 0, len(agents))
	for _, ag := range agents {
		views = append(views, fromAgent(ag))
	}
	dto.ResponseSuccess(c, views)
}

// ActivateAgent 激活指定代理。
func (h *Handler) ActivateAgent(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, nethttp.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}

	agentID, err := uuid.Parse(c.Param("agent_id"))
	if err != nil {
		dto.ResponseError(c, nethttp.StatusBadRequest, "invalid agent_id", err)
		return
	}

	var req activateAgentRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	result, err := h.service.Activate(c.Request.Context(), agent_lifecycle.ActivateInput{
		AgentID:     agentID,
		TenantID:    req.TenantID,
		Reason:      req.Reason,
		RequestedBy: req.RequestedBy,
		TraceID:     req.TraceID,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, fromAgent(result))
}

func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agent_lifecycle.ErrAliasConflict):
		dto.ResponseError(c, nethttp.StatusConflict, "agent alias conflict", err)
	case errors.Is(err, agent_lifecycle.ErrAgentNotFound):
		dto.ResponseError(c, nethttp.StatusNotFound, "agent not found", err)
	case errors.Is(err, agent_lifecycle.ErrInvalidStatusTransition):
		dto.ResponseError(c, nethttp.StatusConflict, "invalid status transition", err)
	default:
		dto.ResponseError(c, nethttp.StatusInternalServerError, "internal error", err)
	}
}
