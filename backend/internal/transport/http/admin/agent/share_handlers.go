package agent

import (
	"errors"
	nethttp "net/http"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ShareHandler 提供共享/撤销 HTTP API。
type ShareHandler struct {
	service *agent_lifecycle.Service
}

// NewShareHandler 构造 ShareHandler。
func NewShareHandler(deps *shared.Deps) *ShareHandler {
	if deps == nil || deps.AgentLifecycle == nil || deps.AgentLifecycle.Service == nil {
		return &ShareHandler{}
	}
	return &ShareHandler{service: deps.AgentLifecycle.Service}
}

// CreateShare 为指定 Agent 创建共享记录。
func (h *ShareHandler) CreateShare(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, nethttp.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	agentIDStr := c.Param("agent_id")
	if agentIDStr == "" {
		agentIDStr = c.Param("id")
	}
	if agentIDStr == "" {
		agentIDStr = c.Param("uuid")
	}
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		dto.ResponseError(c, nethttp.StatusBadRequest, "invalid agent_id", err)
		return
	}
	var req shareAgentRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	input := agent_lifecycle.ShareInput{
		AgentID:     agentID,
		TenantUUID:  req.TenantUUID,
		Quotas:      toShareQuotas(req.Quotas),
		Metadata:    req.Metadata,
		RequestedBy: req.RequestedBy,
		TraceID:     req.TraceID,
	}
	share, err := h.service.ShareAgent(c.Request.Context(), input)
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, nethttp.StatusCreated, fromShare(share))
}

// RevokeShare 撤销共享记录。
func (h *ShareHandler) RevokeShare(c *gin.Context) {
	if h.service == nil {
		dto.ResponseError(c, nethttp.StatusServiceUnavailable, "agent lifecycle service not available", nil)
		return
	}
	shareID, err := uuid.Parse(c.Param("share_id"))
	if err != nil {
		dto.ResponseError(c, nethttp.StatusBadRequest, "invalid share_id", err)
		return
	}
	var req revokeShareRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	share, err := h.service.RevokeAgentShare(c.Request.Context(), agent_lifecycle.RevokeShareInput{
		ShareID:     shareID,
		Reason:      req.Reason,
		RequestedBy: req.RequestedBy,
		TraceID:     req.TraceID,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, fromShare(share))
}

func (h *ShareHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agent_lifecycle.ErrAgentNotFound):
		dto.ResponseError(c, nethttp.StatusNotFound, "agent not found", err)
	case errors.Is(err, agent_lifecycle.ErrAgentShareExists):
		dto.ResponseError(c, nethttp.StatusConflict, "agent already shared to tenant", err)
	case errors.Is(err, agent_lifecycle.ErrAgentShareNotFound):
		dto.ResponseError(c, nethttp.StatusNotFound, "share not found", err)
	case errors.Is(err, agent_lifecycle.ErrShareValidationFailed):
		dto.ResponseError(c, nethttp.StatusBadRequest, err.Error(), err)
	default:
		dto.ResponseError(c, nethttp.StatusInternalServerError, "internal error", err)
	}
}

type shareAgentRequest struct {
	TenantUUID  string            `json:"tenant_uuid" binding:"required,uuid4"`
	RequestedBy string            `json:"requested_by"`
	TraceID     string            `json:"trace_id"`
	Quotas      []shareQuotaDTO   `json:"quotas"`
	Metadata    map[string]string `json:"metadata"`
}

type shareQuotaDTO struct {
	Type  string `json:"type" binding:"required"`
	Limit int32  `json:"limit"`
}

type revokeShareRequest struct {
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
	TraceID     string `json:"trace_id"`
}

type shareResponse struct {
	ID         string            `json:"id"`
	AgentID    string            `json:"agent_id"`
	TenantUUID string            `json:"tenant_uuid"`
	Status     string            `json:"status"`
	Quotas     []shareQuotaDTO   `json:"quotas,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
	RevokedAt  *string           `json:"revoked_at,omitempty"`
	Reason     string            `json:"reason,omitempty"`
}

func toShareQuotas(items []shareQuotaDTO) []agent_lifecycle.ShareQuota {
	if len(items) == 0 {
		return nil
	}
	quotas := make([]agent_lifecycle.ShareQuota, 0, len(items))
	for _, item := range items {
		quotas = append(quotas, agent_lifecycle.ShareQuota{
			Type:  item.Type,
			Limit: item.Limit,
		})
	}
	return quotas
}

func fromShare(share *agent_lifecycle.AgentShare) shareResponse {
	if share == nil {
		return shareResponse{}
	}
	quotas := make([]shareQuotaDTO, 0, len(share.Quotas))
	for _, item := range share.Quotas {
		quotas = append(quotas, shareQuotaDTO{
			Type:  item.Type,
			Limit: item.Limit,
		})
	}
	resp := shareResponse{
		ID:         share.ID.String(),
		AgentID:    share.AgentID.String(),
		TenantUUID: share.TenantUUID,
		Status:     share.Status,
		Quotas:     quotas,
		Metadata:   share.Metadata,
		Reason:     share.Reason,
	}
	if share.CreatedAt != "" {
		resp.CreatedAt = share.CreatedAt
	} else {
		resp.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if share.UpdatedAt != "" {
		resp.UpdatedAt = share.UpdatedAt
	} else {
		resp.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	resp.RevokedAt = share.RevokedAt
	return resp
}
