package runtime

import (
	"context"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// EventHooks observes deployment lifecycle transitions.
type EventHooks interface {
	OnCanaryStarted(ctx context.Context, payload map[string]any)
	OnCanaryProgress(ctx context.Context, payload map[string]any)
	OnCanaryCompleted(ctx context.Context, payload map[string]any)
	OnCanaryRolledBack(ctx context.Context, payload map[string]any)
}

// NewEventHooks builds event hooks backed by EventBus or noop.
func NewEventHooks(bus event_bus.EventBus) EventHooks {
	if bus == nil {
		return noopHooks{}
	}
	return &busHooks{bus: bus}
}

type noopHooks struct{}

func (noopHooks) OnCanaryStarted(context.Context, map[string]any)    {}
func (noopHooks) OnCanaryProgress(context.Context, map[string]any)   {}
func (noopHooks) OnCanaryCompleted(context.Context, map[string]any)  {}
func (noopHooks) OnCanaryRolledBack(context.Context, map[string]any) {}

type busHooks struct {
	bus event_bus.EventBus
}

func (h *busHooks) publish(ctx context.Context, topic string, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	if _, ok := payload["timestamp"]; !ok {
		payload["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	h.bus.Publish(topic, payload, ctx)
}

func (h *busHooks) OnCanaryStarted(ctx context.Context, payload map[string]any) {
	h.publish(ctx, "plugin.release.canary.started", payload)
}

func (h *busHooks) OnCanaryProgress(ctx context.Context, payload map[string]any) {
	h.publish(ctx, "plugin.release.canary.progress", payload)
}

func (h *busHooks) OnCanaryCompleted(ctx context.Context, payload map[string]any) {
	h.publish(ctx, "plugin.release.canary.completed", payload)
}

func (h *busHooks) OnCanaryRolledBack(ctx context.Context, payload map[string]any) {
	h.publish(ctx, "plugin.release.canary.rolled_back", payload)
}
