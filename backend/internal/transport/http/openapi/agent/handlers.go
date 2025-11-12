package agent

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler 负责 OpenAPI 侧的健康查询。
type Handler struct {
	service *agent_lifecycle.Service
}

// NewHandler 构造 Handler。
func NewHandler(service *agent_lifecycle.Service) *Handler {
	return &Handler{service: service}
}

// GetHealthSummary 返回最新健康摘要。
func (h *Handler) GetHealthSummary(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	agentID, err := uuid.Parse(c.Param("agent_id"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid agent_id", err)
		return
	}
	summary, err := h.service.GetHealthSummary(c.Request.Context(), agentID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, fromHealthSummary(summary))
}

// ListHealthHistory 返回健康历史。
func (h *Handler) ListHealthHistory(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	agentID, err := uuid.Parse(c.Param("agent_id"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid agent_id", err)
		return
	}
	rangeHours, _ := strconv.Atoi(c.DefaultQuery("range_hours", "24"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	snapshots, err := h.service.ListHealthSnapshots(c.Request.Context(), agentID, rangeHours, limit)
	if err != nil {
		h.handleError(c, err)
		return
	}

	resp := healthHistoryResponse{Snapshots: make([]healthSummaryResponse, 0, len(snapshots))}
	for _, snap := range snapshots {
		resp.Snapshots = append(resp.Snapshots, fromHealthSummary(snap))
	}
	dto.ResponseSuccess(c, resp)
}

func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agent_lifecycle.ErrAgentNotFound):
		dto.ResponseError(c, http.StatusNotFound, "agent not found", err)
	case errors.Is(err, agent_lifecycle.ErrInvalidStatusTransition):
		dto.ResponseError(c, http.StatusConflict, err.Error(), err)
	case errors.Is(err, agent_lifecycle.ErrInvalidCapacity), errors.Is(err, agent_lifecycle.ErrCapacityExceeded):
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "internal error", err)
	}
}
