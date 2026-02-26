package eventfabric

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	internalbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	deliverysvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminRetryTaskHandler struct {
	deps *shared.Deps
}

type createRetryTaskRequest struct {
	Topic        string         `json:"topic"`
	SubscriberID string         `json:"subscriber_id"`
	Reason       string         `json:"reason"`
	Immediate    *bool          `json:"immediate"`
	Payload      map[string]any `json:"payload"`
}

func NewAdminRetryTaskHandler(deps *shared.Deps) *AdminRetryTaskHandler {
	return &AdminRetryTaskHandler{deps: deps}
}

type retryTaskStatusDTO struct {
	DeliveryID    string     `json:"delivery_id"`
	EventID       string     `json:"event_id"`
	Topic         string     `json:"topic"`
	SubscriberID  string     `json:"subscriber_id"`
	TenantKey     string     `json:"tenant_key"`
	Status        string     `json:"status"`
	LastErrorCode string     `json:"last_error_code"`
	NackReason    string     `json:"nack_reason"`
	ScheduledAt   *time.Time `json:"scheduled_at,omitempty"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	AckedAt       *time.Time `json:"acked_at,omitempty"`
}

func (h *AdminRetryTaskHandler) CreateTask(c *gin.Context) {
	if h == nil || h.deps == nil || h.deps.EventFabric == nil || h.deps.EventFabric.Delivery == nil || h.deps.DB == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("retry service unavailable", nil))
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	var req createRetryTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid request payload", err))
		return
	}

	topic := strings.TrimSpace(req.Topic)
	if topic == "" {
		topic = internalbus.TopicSystemNotification
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "manual retry sample"
	}

	eventID := fmt.Sprintf("retry.seed.%s.%d", strings.TrimSpace(tenantUUID), time.Now().UTC().UnixMilli())
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	payload := req.Payload
	if payload == nil {
		payload = map[string]any{
			"source": "monitor.retry-seed",
			"ts":     time.Now().UTC().Format(time.RFC3339),
		}
	}
	payloadBytes, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", marshalErr))
		return
	}

	publishErr := h.deps.EventFabric.Delivery.Publish(c.Request.Context(), deliverysvc.PublishRequest{
		TenantUUID:    tenantUUID,
		Topic:         topic,
		EventID:       eventID,
		TraceID:       traceID,
		Version:       "v1",
		Payload:       payloadBytes,
		PayloadFormat: "json",
		Attributes: map[string]string{
			"source": "admin.event_fabric.retry.seed",
		},
	})
	if publishErr != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("publish retry seed failed", publishErr))
		return
	}

	var attempts []eventfabricmodel.DeliveryAttempt
	if err := h.deps.DB.WithContext(c.Request.Context()).
		Where("tenant_key = ? AND event_id = ?", strings.TrimSpace(tenantUUID), eventID).
		Order("created_at ASC").
		Find(&attempts).Error; err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("query delivery attempts failed", err))
		return
	}
	if len(attempts) == 0 {
		dto.RespondErrorFrom(c, dto.NewNotFound("no delivery attempts generated for retry seed", nil))
		return
	}

	targetSubscriber := strings.TrimSpace(req.SubscriberID)
	if targetSubscriber == "" {
		targetSubscriber = attempts[0].SubscriberID
	}

	var targetAttempt *eventfabricmodel.DeliveryAttempt
	for idx := range attempts {
		if strings.TrimSpace(attempts[idx].SubscriberID) == targetSubscriber {
			targetAttempt = &attempts[idx]
			break
		}
	}
	if targetAttempt == nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("subscriber not found in generated attempts", nil))
		return
	}

	plan, nackErr := h.deps.EventFabric.Delivery.Nack(
		c.Request.Context(),
		targetAttempt.UUID.String(),
		targetAttempt.SubscriberID,
		reason,
	)
	if nackErr != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("create retry seed failed", nackErr))
		return
	}

	immediate := false
	if req.Immediate != nil {
		immediate = *req.Immediate
	}
	retryAt := time.Now().UTC().Add(plan.NextDelay)
	if immediate {
		if h.deps.EventFabric != nil && h.deps.EventFabric.Scheduler != nil {
			_, scheduleErr := h.deps.EventFabric.Scheduler.Schedule(c.Request.Context(), deliverysvc.ScheduleOptions{
				TenantKey:    targetAttempt.TenantKey,
				SubscriberID: targetAttempt.SubscriberID,
				EventID:      targetAttempt.EventID,
				EnvelopeUUID: targetAttempt.EnvelopeUUID.String(),
				Attempt:      targetAttempt.DeliveryNo,
				BaseDelay:    0,
				Metadata: map[string]string{
					"attempt_uuid": targetAttempt.UUID.String(),
				},
			})
			if scheduleErr != nil {
				dto.RespondErrorFrom(c, dto.NewInternal("schedule retry seed immediate failed", scheduleErr))
				return
			}
		}
		retryAt = time.Now().UTC()
	}

	dto.ResponseSuccess(c, gin.H{
		"event_id":            eventID,
		"delivery_id":         targetAttempt.UUID.String(),
		"topic":               topic,
		"subscriber_id":       targetAttempt.SubscriberID,
		"tenant_key":          targetAttempt.TenantKey,
		"retry_after_seconds": int(plan.NextDelay.Seconds()),
		"retry_at":            retryAt.Format(time.RFC3339),
		"immediate":           immediate,
		"remaining_attempts":  plan.RemainingAttempts,
		"max_attempts":        plan.MaxAttempts,
		"trace_id":            traceID,
	})
}

func (h *AdminRetryTaskHandler) GetTask(c *gin.Context) {
	if h == nil || h.deps == nil || h.deps.DB == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("retry service unavailable", nil))
		return
	}
	deliveryID := strings.TrimSpace(c.Param("delivery_id"))
	if deliveryID == "" {
		dto.RespondErrorFrom(c, dto.NewBadRequest("delivery_id required", nil))
		return
	}
	deliveryUUID, err := uuid.Parse(deliveryID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid delivery_id", err))
		return
	}

	var attempt eventfabricmodel.DeliveryAttempt
	if err := h.deps.DB.WithContext(c.Request.Context()).
		Where("uuid = ?", deliveryUUID).
		First(&attempt).Error; err != nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("retry delivery not found", err))
		return
	}

	var envelope eventfabricmodel.EventEnvelope
	_ = h.deps.DB.WithContext(c.Request.Context()).
		Where("uuid = ?", attempt.EnvelopeUUID).
		First(&envelope).Error

	var topic eventfabricmodel.TopicDefinition
	topicFullName := ""
	if envelope.TopicUUID != uuid.Nil {
		if err := h.deps.DB.WithContext(c.Request.Context()).
			Where("uuid = ?", envelope.TopicUUID).
			First(&topic).Error; err == nil {
			topicFullName = strings.TrimSpace(topic.FullTopic)
		}
	}

	dto.ResponseSuccess(c, retryTaskStatusDTO{
		DeliveryID:    attempt.UUID.String(),
		EventID:       attempt.EventID,
		Topic:         topicFullName,
		SubscriberID:  attempt.SubscriberID,
		TenantKey:     attempt.TenantKey,
		Status:        attempt.Status,
		LastErrorCode: attempt.LastErrorCode,
		NackReason:    attempt.NackReason,
		ScheduledAt:   attempt.ScheduledAt,
		LastAttemptAt: attempt.LastAttemptAt,
		AckedAt:       attempt.AckedAt,
	})
}
