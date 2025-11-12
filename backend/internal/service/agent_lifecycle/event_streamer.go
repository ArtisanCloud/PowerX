package agent_lifecycle

import (
	"context"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// EventStreamer 负责向 StateBus 推送标准化事件。
type EventStreamer struct {
	bus    event_bus.EventBus
	topics StateBusTopics
	clock  func() time.Time
}

// NewEventStreamer 构造 EventStreamer。
func NewEventStreamer(bus event_bus.EventBus, topics StateBusTopics, clock func() time.Time) *EventStreamer {
	if clock == nil {
		clock = time.Now
	}
	return &EventStreamer{
		bus:    bus,
		topics: topics,
		clock:  clock,
	}
}

// EmitLifecycle 推送生命周期事件。
func (e *EventStreamer) EmitLifecycle(ctx context.Context, action string, payload map[string]any, traceID string) {
	e.emit(ctx, e.topics.Lifecycle, "agent.lifecycle."+action, payload, traceID)
}

// EmitHealth 推送健康事件。
func (e *EventStreamer) EmitHealth(ctx context.Context, status string, payload map[string]any, traceID string) {
	e.emit(ctx, e.topics.Health, "agent.health."+status, payload, traceID)
}

func (e *EventStreamer) emit(ctx context.Context, topic, event string, payload map[string]any, traceID string) {
	if e == nil || e.bus == nil {
		return
	}
	if topic == "" {
		return
	}
	envelope := map[string]any{
		"event":     event,
		"source":    "agent_lifecycle",
		"version":   "v1",
		"trace_id":  traceID,
		"timestamp": e.clock().UTC().Format(time.RFC3339Nano),
		"payload":   clonePayload(payload),
	}
	e.bus.Publish(topic, envelope, ctx)
}

func clonePayload(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
