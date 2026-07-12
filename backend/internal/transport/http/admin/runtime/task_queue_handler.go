package runtime

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
)

type taskQueueHandler struct {
	driver event_bus.TaskDriver
	guard  func(ctx context.Context, pluginID string) error
}

type taskQueueMessageDTO struct {
	ID             string            `json:"id"`
	TenantKey      string            `json:"tenant_key"`
	SubscriberID   string            `json:"subscriber_id"`
	Topic          string            `json:"topic"`
	PayloadBase64  string            `json:"payload_base64"`
	Headers        map[string]string `json:"headers,omitempty"`
	Attempt        int               `json:"attempt,omitempty"`
	TraceID        string            `json:"trace_id,omitempty"`
	VisibleAt      *time.Time        `json:"visible_at,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
}

type taskQueueEnqueueRequest struct {
	Message taskQueueMessageDTO `json:"message"`
}

type taskQueueDequeueRequest struct {
	TenantKey     string `json:"tenant_key"`
	SubscriberID  string `json:"subscriber_id"`
	MaxItems      int    `json:"max_items"`
	WaitTimeoutMS int64  `json:"wait_timeout_ms"`
}

type taskQueueAckRequest struct {
	TenantKey    string            `json:"tenant_key"`
	SubscriberID string            `json:"subscriber_id"`
	MessageID    string            `json:"message_id"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type taskQueueNackRequest struct {
	TenantKey    string            `json:"tenant_key"`
	SubscriberID string            `json:"subscriber_id"`
	MessageID    string            `json:"message_id"`
	Reason       string            `json:"reason"`
	RetryAt      *time.Time        `json:"retry_at,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type taskQueueRetryRequest struct {
	Message taskQueueMessageDTO `json:"message"`
	RetryAt *time.Time          `json:"retry_at,omitempty"`
	Reason  string              `json:"reason"`
}

func newTaskQueueHandler(deps *shared.Deps) *taskQueueHandler {
	if deps == nil || deps.EventFabric == nil || deps.EventFabric.TaskDriver == nil {
		return nil
	}
	var guard func(ctx context.Context, pluginID string) error
	if deps.DB != nil {
		guard = pluginservice.NewPluginDrainJobService(deps.DB).EnsurePluginAcceptsNewUsage
	}
	return &taskQueueHandler{driver: deps.EventFabric.TaskDriver, guard: guard}
}

func (h *taskQueueHandler) enqueue(c *gin.Context) {
	var req taskQueueEnqueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	msg, err := req.Message.toTaskMessage(requireTenantKey(c, req.Message.TenantKey))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid message", err)
		return
	}
	if err := h.ensurePluginAcceptsNewUsage(c, msg.Metadata); err != nil {
		dto.ResponseError(c, dto.StatusCode(err), dto.MessageOf(err), err)
		return
	}
	if err := h.driver.Enqueue(c.Request.Context(), msg); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "enqueue task failed", err)
		return
	}
	dto.ResponseSuccessWithStatusAndPayload(c, http.StatusOK, map[string]any{"ok": true, "message_id": msg.ID})
}

func (h *taskQueueHandler) dequeue(c *gin.Context) {
	var req taskQueueDequeueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	tenantKey := requireTenantKey(c, req.TenantKey)
	wait := time.Duration(req.WaitTimeoutMS) * time.Millisecond
	messages, err := h.driver.Dequeue(c.Request.Context(), event_bus.DequeueRequest{
		TenantKey:    tenantKey,
		SubscriberID: strings.TrimSpace(req.SubscriberID),
		MaxItems:     req.MaxItems,
		WaitTimeout:  wait,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "dequeue task failed", err)
		return
	}
	out := make([]taskQueueMessageDTO, 0, len(messages))
	for _, msg := range messages {
		out = append(out, taskMessageToDTO(msg))
	}
	dto.ResponseSuccessWithStatusAndPayload(c, http.StatusOK, map[string]any{"messages": out})
}

func (h *taskQueueHandler) ack(c *gin.Context) {
	var req taskQueueAckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	if err := h.driver.Ack(c.Request.Context(), event_bus.AckRequest{
		TenantKey:    requireTenantKey(c, req.TenantKey),
		SubscriberID: strings.TrimSpace(req.SubscriberID),
		MessageID:    strings.TrimSpace(req.MessageID),
		Metadata:     req.Metadata,
	}); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "ack task failed", err)
		return
	}
	dto.ResponseSuccessWithStatusAndPayload(c, http.StatusOK, map[string]any{"ok": true})
}

func (h *taskQueueHandler) nack(c *gin.Context) {
	var req taskQueueNackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	var retryAt time.Time
	if req.RetryAt != nil {
		retryAt = req.RetryAt.UTC()
	}
	if err := h.driver.Nack(c.Request.Context(), event_bus.NackRequest{
		TenantKey:    requireTenantKey(c, req.TenantKey),
		SubscriberID: strings.TrimSpace(req.SubscriberID),
		MessageID:    strings.TrimSpace(req.MessageID),
		Reason:       strings.TrimSpace(req.Reason),
		RetryAt:      retryAt,
		Metadata:     req.Metadata,
	}); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "nack task failed", err)
		return
	}
	dto.ResponseSuccessWithStatusAndPayload(c, http.StatusOK, map[string]any{"ok": true})
}

func (h *taskQueueHandler) retry(c *gin.Context) {
	var req taskQueueRetryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	msg, err := req.Message.toTaskMessage(requireTenantKey(c, req.Message.TenantKey))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid message", err)
		return
	}
	if err := h.ensurePluginAcceptsNewUsage(c, msg.Metadata); err != nil {
		dto.ResponseError(c, dto.StatusCode(err), dto.MessageOf(err), err)
		return
	}
	var retryAt time.Time
	if req.RetryAt != nil {
		retryAt = req.RetryAt.UTC()
	}
	if err := h.driver.Retry(c.Request.Context(), event_bus.RetryRequest{
		Message: msg,
		RetryAt: retryAt,
		Reason:  strings.TrimSpace(req.Reason),
	}); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "retry task failed", err)
		return
	}
	dto.ResponseSuccessWithStatusAndPayload(c, http.StatusOK, map[string]any{"ok": true, "message_id": msg.ID})
}

func (h *taskQueueHandler) ensurePluginAcceptsNewUsage(c *gin.Context, metadata map[string]string) error {
	if h == nil || h.guard == nil || len(metadata) == 0 {
		return nil
	}
	pluginID := strings.TrimSpace(metadata["plugin_id"])
	if pluginID == "" {
		return nil
	}
	if c == nil || c.Request == nil {
		return dto.NewErrorWithCode(http.StatusBadRequest, pluginservice.ErrCodePluginDrainInvalidRequest, "缺少请求上下文", errors.New("request context required"))
	}
	return h.guard(c.Request.Context(), pluginID)
}

func (m taskQueueMessageDTO) toTaskMessage(defaultTenantKey string) (event_bus.TaskMessage, error) {
	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(m.PayloadBase64))
	if err != nil {
		return event_bus.TaskMessage{}, err
	}
	tenantKey := strings.TrimSpace(m.TenantKey)
	if tenantKey == "" {
		tenantKey = strings.TrimSpace(defaultTenantKey)
	}
	var visibleAt time.Time
	if m.VisibleAt != nil {
		visibleAt = m.VisibleAt.UTC()
	}
	return event_bus.TaskMessage{
		ID:           strings.TrimSpace(m.ID),
		TenantKey:    tenantKey,
		SubscriberID: strings.TrimSpace(m.SubscriberID),
		Topic:        strings.TrimSpace(m.Topic),
		Payload:      payload,
		Headers:      m.Headers,
		Attempt:      m.Attempt,
		TraceID:      strings.TrimSpace(m.TraceID),
		VisibleAt:    visibleAt,
		Metadata:     m.Metadata,
	}, nil
}

func taskMessageToDTO(msg event_bus.TaskMessage) taskQueueMessageDTO {
	var visibleAt *time.Time
	if !msg.VisibleAt.IsZero() {
		v := msg.VisibleAt.UTC()
		visibleAt = &v
	}
	return taskQueueMessageDTO{
		ID:            strings.TrimSpace(msg.ID),
		TenantKey:     strings.TrimSpace(msg.TenantKey),
		SubscriberID:  strings.TrimSpace(msg.SubscriberID),
		Topic:         strings.TrimSpace(msg.Topic),
		PayloadBase64: base64.StdEncoding.EncodeToString(msg.Payload),
		Headers:       msg.Headers,
		Attempt:       msg.Attempt,
		TraceID:       strings.TrimSpace(msg.TraceID),
		VisibleAt:     visibleAt,
		Metadata:      msg.Metadata,
	}
}

func requireTenantKey(c *gin.Context, fallback string) string {
	if strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback)
	}
	if tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c); err == nil && strings.TrimSpace(tenantUUID) != "" {
		return strings.TrimSpace(tenantUUID)
	}
	return ""
}
