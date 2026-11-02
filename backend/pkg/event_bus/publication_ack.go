package event_bus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func publicationEvent(eventType string, payload interface{}, ctx context.Context) Event {
	e := Event{Name: eventType, Payload: payload, Ctx: ctx, ID: uuid.NewString()}
	if ctx != nil {
		if trace, ok := ctx.Value("trace_id").(string); ok {
			e.TraceID = trace
		}
		e.TenantUUID = tenantUUIDFromContext(ctx)
	}
	return e
}
func (reb *redisEventBus) PublishAcknowledged(eventType string, payload interface{}, ctx context.Context) (PublicationReceipt, error) {
	var out PublicationReceipt
	if ctx == nil {
		ctx = context.Background()
	}
	reb.handlersMu.RLock()
	closed := reb.closed
	reb.handlersMu.RUnlock()
	if closed {
		return out, errors.New("event_bus_closed")
	}
	e := publicationEvent(eventType, payload, ctx)
	raw, err := json.Marshal(e)
	if err != nil {
		return out, err
	}
	run, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	n, err := reb.client.Publish(run, fmt.Sprintf("corex:event:%s", eventType), raw).Result()
	if err != nil {
		return out, err
	}
	status := "published"
	if n == 0 {
		status = "unrouted"
	}
	return PublicationReceipt{EventID: e.ID, Status: status, Subscribers: n}, nil
}
func (leb *localEventBus) PublishAcknowledged(eventType string, payload interface{}, ctx context.Context) (PublicationReceipt, error) {
	var out PublicationReceipt
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	leb.mu.RLock()
	if leb.closed {
		leb.mu.RUnlock()
		return out, errors.New("event_bus_closed")
	}
	handlers := append([]Handler(nil), leb.subscribers[eventType]...)
	leb.mu.RUnlock()
	e := publicationEvent(eventType, payload, ctx)
	for _, handler := range handlers {
		go func(h Handler) {
			run, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			event := e
			event.Ctx = run
			defer func() {
				if recover() != nil {
					logger.ErrorF(ctx, "本地事件处理器失败: %s", eventType)
				}
			}()
			if err := h(event); err != nil {
				logger.ErrorF(ctx, "本地事件订阅者处理失败: %s", eventType)
			}
		}(handler)
	}
	status := "dispatched"
	if len(handlers) == 0 {
		status = "unrouted"
	}
	return PublicationReceipt{EventID: e.ID, Status: status, Subscribers: int64(len(handlers))}, nil
}
