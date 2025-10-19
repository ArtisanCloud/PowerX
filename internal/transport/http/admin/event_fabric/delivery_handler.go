package eventfabric

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	sharedsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type AdminDeliveryHandlerOptions struct {
	Service delivery.Service
}

type AdminDeliveryHandler struct {
	service delivery.Service
}

func NewAdminDeliveryHandler(opts AdminDeliveryHandlerOptions) *AdminDeliveryHandler {
	return &AdminDeliveryHandler{service: opts.Service}
}

type publishEventRequest struct {
	TenantID       string            `json:"tenant_id" validate:"required"`
	Topic          string            `json:"topic" validate:"required"`
	EventID        string            `json:"event_id" validate:"required"`
	TraceID        string            `json:"trace_id"`
	Version        string            `json:"version" validate:"required"`
	Payload        string            `json:"payload" validate:"required"`
	PayloadFormat  string            `json:"payload_format"`
	IdempotencyKey string            `json:"idempotency_key"`
	Attributes     map[string]string `json:"attributes"`
}

func (h *AdminDeliveryHandler) PublishEvent(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("delivery service unavailable", nil))
		return
	}

	var req publishEventRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid request payload", err))
		return
	}

	payloadBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.Payload))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("payload must be base64 encoded", err))
		return
	}

	attributes := make(map[string]string, len(req.Attributes))
	for k, v := range req.Attributes {
		if key := strings.TrimSpace(k); key != "" {
			attributes[key] = v
		}
	}

	if err := h.service.Publish(c.Request.Context(), delivery.PublishRequest{
		TenantID:       strings.TrimSpace(req.TenantID),
		Topic:          strings.TrimSpace(req.Topic),
		EventID:        strings.TrimSpace(req.EventID),
		TraceID:        strings.TrimSpace(req.TraceID),
		Version:        strings.TrimSpace(req.Version),
		Payload:        payloadBytes,
		PayloadFormat:  strings.TrimSpace(req.PayloadFormat),
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		Attributes:     attributes,
	}); err != nil {
		dto.RespondErrorFrom(c, mapDeliveryError(err))
		return
	}

	c.Status(http.StatusAccepted)
}

func mapDeliveryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sharedsvc.ErrTenantMismatch):
		return dto.NewBadRequest("tenant mismatch", err)
	case errors.Is(err, sharedsvc.ErrUnauthorized):
		return dto.NewForbidden("unauthorized", err)
	case errors.Is(err, sharedsvc.ErrAckTimeout):
		return dto.NewConflict("ack timeout", err)
	case errors.Is(err, sharedsvc.ErrRetryExhausted):
		return dto.NewConflict("retry exhausted", err)
	default:
		return dto.NewInternal("delivery internal error", err)
	}
}
