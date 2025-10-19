package eventfabric

import (
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/dlq"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type AdminDLQHandlerOptions struct {
	Service dlq.Service
}

type AdminDLQHandler struct {
	service dlq.Service
}

func NewAdminDLQHandler(opts AdminDLQHandlerOptions) *AdminDLQHandler {
	return &AdminDLQHandler{service: opts.Service}
}

func (h *AdminDLQHandler) ListMessages(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("dlq service unavailable", nil))
		return
	}

	tenantID := strings.TrimSpace(c.Query("tenant_id"))
	if tenantID == "" {
		dto.RespondErrorFrom(c, dto.NewBadRequest("tenant_id is required", nil))
		return
	}
	topic := strings.TrimSpace(c.Query("topic"))
	status := strings.TrimSpace(c.Query("status"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize <= 0 {
		pageSize = 20
	}

	messages, total, err := h.service.List(c.Request.Context(), dlq.ListRequest{
		TenantID: tenantID,
		TopicID:  topic,
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list dlq messages failed", err))
		return
	}

	items := make([]gin.H, 0, len(messages))
	for _, msg := range messages {
		items = append(items, gin.H{
			"id":          msg.ID,
			"tenant_id":   msg.TenantID,
			"topic":       msg.Topic,
			"event_id":    msg.EventID,
			"retry_count": msg.RetryCount,
			"last_error":  msg.LastError,
			"failed_at":   msg.CreatedAt.Format(time.RFC3339),
		})
	}

	dto.ResponseSuccess(c, gin.H{
		"items": items,
		"pagination": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

type replayRequest struct {
	MessageIDs []string `json:"message_ids" validate:"required,min=1,dive,required"`
	OperatorID string   `json:"operator_id"`
	Notes      string   `json:"notes"`
}

func (h *AdminDLQHandler) ReplayMessages(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("dlq service unavailable", nil))
		return
	}

	var req replayRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid request payload", err))
		return
	}

	count, err := h.service.Replay(c.Request.Context(), dlq.ReplayRequest{
		MessageIDs: req.MessageIDs,
		OperatorID: strings.TrimSpace(req.OperatorID),
		Notes:      strings.TrimSpace(req.Notes),
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("replay dlq messages failed", err))
		return
	}

	dto.ResponseSuccess(c, gin.H{"replayed": count})
}

func (h *AdminDLQHandler) PurgeMessages(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("dlq service unavailable", nil))
		return
	}

	tenantID := strings.TrimSpace(c.Query("tenant_id"))
	if tenantID == "" {
		dto.RespondErrorFrom(c, dto.NewBadRequest("tenant_id is required", nil))
		return
	}

	topic := strings.TrimSpace(c.Query("topic"))
	removed, err := h.service.Purge(c.Request.Context(), tenantID, topic)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("purge dlq messages failed", err))
		return
	}

	dto.ResponseSuccess(c, gin.H{"removed": removed})
}
