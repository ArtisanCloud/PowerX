package shared

import (
	"context"
	"strings"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	replayservice "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/replay"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

type replayTaskWSStatusEmitter struct{}

func newReplayTaskWSStatusEmitter() replayservice.StatusEmitter {
	return &replayTaskWSStatusEmitter{}
}

func (e *replayTaskWSStatusEmitter) EmitReplayTaskStatus(ctx context.Context, event replayservice.ReplayTaskStatusEvent) {
	if e == nil {
		return
	}
	tenantKey := strings.TrimSpace(event.TenantKey)
	if tenantKey == "" {
		return
	}
	payload := map[string]any{
		"kind": eventbus.NotificationKindEventFabricReplayTask,
		"data": event,
	}
	wsbus.DefaultHub.Publish(tenantKey, eventbus.TopicSystemNotification, payload, reqctx.GetTraceID(ctx))
}
