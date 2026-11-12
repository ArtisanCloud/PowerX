package notifications

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	notificationssvc "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	"github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) createIdempotent(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if err := h.svc.AuthorizeCreate(c.Request.Context()); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var input notificationssvc.IdempotentCreateInput
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		dto.ResponseError(c, 400, "NOTIFICATION_CREATE_INVALID", nil)
		return
	}
	receipt, err := h.svc.CreateIdempotent(c.Request.Context(), input)
	if err != nil {
		var coded *dto.AppError
		if errors.As(err, &coded) {
			dto.RespondErrorFrom(c, err)
		} else {
			dto.ResponseError(c, 503, "NOTIFICATION_UNAVAILABLE", nil)
		}
		return
	}
	if !receipt.Replayed {
		tenant := reqctx.GetTenantUUID(c.Request.Context())
		bus.DefaultHub.PublishWithContext(c.Request.Context(), tenant, eventbus.TopicSystemNotification, receipt.Notification, reqctx.GetTraceID(c.Request.Context()))
	}
	dto.ResponseSuccess(c, receipt)
}
