package agent

import (
	"net/http"
	"strconv"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetBridgeState 返回 Agent 的聚合状态与事件时间线。
func (h *Handler) GetBridgeState(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	agentID, err := uuid.Parse(c.Param("agent_id"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid agent_id", err)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	state, err := h.service.GetBridgeState(c.Request.Context(), agentID, limit)
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, fromBridgeState(state))
}

// FreezeAgent 暂停 Agent，供协调/恢复阶段使用。
func (h *Handler) FreezeAgent(c *gin.Context) {
	h.performLifecycleControl(c, "freeze")
}

// RecoverAgent 恢复 Agent。
func (h *Handler) RecoverAgent(c *gin.Context) {
	h.performLifecycleControl(c, "recover")
}

// RebalanceAgent 扩缩容 Agent。
func (h *Handler) RebalanceAgent(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	agentID, err := uuid.Parse(c.Param("agent_id"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid agent_id", err)
		return
	}
	var req bridgeRebalanceRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	result, err := h.service.Scale(c.Request.Context(), agent_lifecycle.ScaleInput{
		AgentID:     agentID,
		TenantID:    req.TenantID,
		Target:      req.TargetCapacityInstances,
		Reason:      req.Reason,
		RequestedBy: req.RequestedBy,
		TraceID:     req.TraceID,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, fromLifecycleResult(result))
}

func (h *Handler) performLifecycleControl(c *gin.Context, action string) {
	if h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	agentID, err := uuid.Parse(c.Param("agent_id"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid agent_id", err)
		return
	}
	var req bridgeControlRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	var result *agent_lifecycle.LifecycleResult
	switch action {
	case "freeze":
		result, err = h.service.Pause(c.Request.Context(), agent_lifecycle.PauseInput{
			AgentID:     agentID,
			TenantID:    req.TenantID,
			Reason:      req.Reason,
			RequestedBy: req.RequestedBy,
			TraceID:     req.TraceID,
		})
	case "recover":
		result, err = h.service.Resume(c.Request.Context(), agent_lifecycle.ResumeInput{
			AgentID:     agentID,
			TenantID:    req.TenantID,
			Reason:      req.Reason,
			RequestedBy: req.RequestedBy,
			TraceID:     req.TraceID,
		})
	default:
		dto.ResponseError(c, http.StatusBadRequest, "unsupported action", nil)
		return
	}
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, fromLifecycleResult(result))
}
