package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func respondMediaError(c *gin.Context, err error) bool {
	var business *dto.AppError
	if errors.As(err, &business) {
		dto.ResponseErrorWithDetails(c, business.HTTPCode, business.Message, business, business.Details)
		return true
	}
	var grant *cap.DirectGrantError
	if errors.As(err, &grant) {
		dto.RespondErrorFrom(c, aisvc.MediaError(grant.Status, grant.Error()))
		return true
	}
	return false
}
func (h *aiHandler) invokeMedia(c *gin.Context, capability string) {
	if h == nil || h.media == nil {
		respondAIError(c, aisvc.MediaError(503, "AI_DEPENDENCY_UNAVAILABLE"))
		return
	}
	// 媒体 v2 不接受请求选择运行环境，运行通道和环境由启动配置/租户设置确定。
	if c.Request.URL.RawQuery != "" || c.GetHeader("X-PowerX-Env") != "" {
		respondAIError(c, aisvc.MediaError(400, "AI_RUNTIME_OVERRIDE_FORBIDDEN"))
		return
	}
	var input aisvc.MediaInvokeOperation
	requestLimit := int64(16 << 20)
	if capability == "image_generation" {
		requestLimit = contract.MediaRequestMaxBytes
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, requestLimit)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	decodeErr := decoder.Decode(&input)
	if decodeErr == nil {
		decodeErr = decoder.Decode(&struct{}{})
		if decodeErr == io.EOF {
			decodeErr = nil
		} else if decodeErr == nil {
			decodeErr = errors.New("trailing JSON")
		}
	}
	if decodeErr != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(decodeErr, &tooLarge) {
			respondAIError(c, aisvc.MediaError(422, "AI_INPUT_LIMIT_EXCEEDED"))
		} else {
			respondAIError(c, aisvc.MediaError(400, "AI_REQUEST_INVALID"))
		}
		return
	}
	if input.Operation != "" && input.Operation != "invoke" {
		respondAIError(c, aisvc.MediaError(400, "AI_OPERATION_INVALID"))
		return
	}
	if key := c.GetHeader("Idempotency-Key"); key != "" {
		if input.IdempotencyKey != "" && input.IdempotencyKey != key {
			respondAIError(c, aisvc.MediaError(409, "AI_IDEMPOTENCY_CONFLICT"))
			return
		}
		input.IdempotencyKey = key
	}
	result, err := h.media.Invoke(c.Request.Context(), capability, input)
	if err != nil {
		respondAIError(c, err)
		return
	}
	if _, ok := result.(*aisvc.MediaTaskView); ok {
		dto.ResponseSuccessWithStatus(c, http.StatusAccepted, result)
	} else {
		dto.ResponseSuccess(c, result)
	}
}
func (h *aiHandler) getMediaTask(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		respondAIError(c, aisvc.MediaError(400, "AI_REQUEST_INVALID"))
		return
	}
	result, err := h.media.Get(c.Request.Context(), c.Param("task_uuid"))
	if err != nil {
		respondAIError(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}
func (h *aiHandler) cancelMediaTask(c *gin.Context) {
	if c.Request.URL.RawQuery != "" || c.Request.ContentLength != 0 {
		respondAIError(c, aisvc.MediaError(400, "AI_REQUEST_INVALID"))
		return
	}
	result, err := h.media.Cancel(c.Request.Context(), c.Param("task_uuid"))
	if err != nil {
		respondAIError(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

// SSE 是状态快照流。恢复时总是返回当前快照，revision 去重；不承诺重放每个中间阶段。
func (h *aiHandler) mediaTaskEvents(c *gin.Context, client *redis.Client) {
	if h == nil || h.media == nil || client == nil {
		respondAIError(c, aisvc.MediaError(503, "AI_EVENTS_UNAVAILABLE"))
		return
	}
	id := c.Param("task_uuid")
	cursor := c.GetHeader("Last-Event-ID")
	if c.Request.URL.RawQuery != "" {
		respondAIError(c, aisvc.MediaError(400, "AI_REQUEST_INVALID"))
		return
	}
	if cursor != "" {
		parts := strings.Split(cursor, ":")
		if len(parts) != 2 || parts[0] != id {
			respondAIError(c, aisvc.MediaError(400, "AI_EVENT_CURSOR_INVALID"))
			return
		}
		if n, e := strconv.ParseInt(parts[1], 10, 64); e != nil || n < 1 {
			respondAIError(c, aisvc.MediaError(400, "AI_EVENT_CURSOR_INVALID"))
			return
		}
	}
	if err := h.media.AuthorizeRead(c.Request.Context()); err != nil {
		respondAIError(c, err)
		return
	}
	deadline := time.Now().Add(5 * time.Minute)
	if claims := reqctx.GetClaims(c.Request.Context()); claims != nil && claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(deadline) {
		deadline = claims.ExpiresAt.Time
	}
	ctx, cancel := context.WithDeadline(c.Request.Context(), deadline)
	defer cancel()
	sub := client.Subscribe(ctx, h.media.EventChannel(reqctx.GetTenantUUID(ctx), id))
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		respondAIError(c, aisvc.MediaError(503, "AI_EVENTS_UNAVAILABLE"))
		return
	}
	initial, err := h.media.Get(ctx, id)
	if err != nil {
		respondAIError(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	controller := http.NewResponseController(c.Writer)
	write := func(event, eventID string, value any) error {
		raw, e := json.Marshal(value)
		if e != nil {
			return e
		}
		_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if eventID != "" {
			if _, e = fmt.Fprintf(c.Writer, "id: %s\n", eventID); e != nil {
				return e
			}
		}
		if _, e = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, raw); e != nil {
			return e
		}
		return controller.Flush()
	}
	last := initial.Revision
	if write("status", fmt.Sprintf("%s:%d", id, last), initial) != nil {
		return
	}
	interval := h.mediaHeartbeat
	if interval <= 0 {
		interval = 15 * time.Second
	}
	heartbeat := time.NewTicker(interval)
	defer heartbeat.Stop()
	// A resubscription may follow missed notifications. Read one new snapshot
	// on its acknowledgement, without periodic task reads.
	changes := sub.ChannelWithSubscriptions()
	for {
		select {
		case <-ctx.Done():
			if c.Request.Context().Err() == nil {
				_ = write("reconnect", "", gin.H{"reason_code": "AI_EVENT_REAUTHORIZE"})
			}
			return
		case change, ok := <-changes:
			if !ok {
				_ = write("reconnect", "", gin.H{"reason_code": "AI_EVENT_CONNECTION_LOST"})
				return
			}
			switch value := change.(type) {
			case *redis.Subscription:
				if value.Kind != "subscribe" {
					continue
				}
			case *redis.Message:
			default:
				continue
			}
			state, e := h.media.Get(ctx, id)
			if e != nil {
				_ = write("error", "", gin.H{"reason_code": "AI_EVENT_ACCESS_REVOKED"})
				return
			}
			if state.Revision > last {
				last = state.Revision
				if write("status", fmt.Sprintf("%s:%d", id, last), state) != nil {
					return
				}
			}
		case <-heartbeat.C:
			if e := h.media.AuthorizeRead(ctx); e != nil {
				_ = write("error", "", gin.H{"reason_code": "AI_EVENT_ACCESS_REVOKED"})
				return
			}
			_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, e := fmt.Fprint(c.Writer, ": keepalive\n\n"); e != nil || controller.Flush() != nil {
				return
			}
		}
	}
}
