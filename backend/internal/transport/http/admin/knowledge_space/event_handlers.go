package knowledge_space

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	event_hotfix "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/event_hotfix"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// EventHandler exposes HTTP endpoints for event hotfix orchestration.
type EventHandler struct {
	svc *event_hotfix.Service
}

func NewEventHandler(deps *shared.Deps) *EventHandler {
	if deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.EventHotfix == nil {
		return nil
	}
	return &EventHandler{svc: deps.KnowledgeSpace.EventHotfix}
}

func (h *EventHandler) Apply(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "event hotfix unavailable", nil)
		return
	}
	var req eventApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	result, err := h.svc.Apply(c.Request.Context(), req.toInput())
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *EventHandler) Retry(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "event hotfix unavailable", nil)
		return
	}
	var req eventApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	result, err := h.svc.Retry(c.Request.Context(), req.toInput())
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *EventHandler) HotUpdate(c *gin.Context) {
	dto.ResponseSuccess(c, gin.H{"status": "enqueued"})
}

func (h *EventHandler) RefreshAgent(c *gin.Context) {
	dto.ResponseSuccess(c, gin.H{"status": "refreshing"})
}

func (h *EventHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, event_hotfix.ErrInvalidEvent):
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
	case errors.Is(err, event_hotfix.ErrPolicyMissing):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	case errors.Is(err, event_hotfix.ErrDuplicateEvent):
		dto.ResponseError(c, http.StatusConflict, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
	}
}

func (req eventApplyRequest) toInput() event_hotfix.ApplyInput {
	received := req.ReceivedAt
	if received == nil || received.IsZero() {
		now := time.Now()
		received = &now
	}
	payload := req.Payload
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["eventType"] = req.EventType
	return event_hotfix.ApplyInput{
		EventID:    strings.TrimSpace(req.EventID),
		EventType:  strings.TrimSpace(req.EventType),
		Payload:    payload,
		ReceivedAt: received.UTC(),
		RetryCount: req.RetryCount,
	}
}
