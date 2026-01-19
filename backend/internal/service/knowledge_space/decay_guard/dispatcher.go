package decay_guard

import (
	"context"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

type eventBusDispatcher struct {
	bus          event_bus.EventBus
	dispatchTopic string
	closeTopic    string
}

func newEventBusDispatcher(bus event_bus.EventBus, dispatchTopic, closeTopic string) TaskDispatcher {
	if bus == nil {
		return nil
	}
	dispatchTopic = strings.TrimSpace(dispatchTopic)
	if dispatchTopic == "" {
		dispatchTopic = "knowledge.decay.task.dispatch"
	}
	closeTopic = strings.TrimSpace(closeTopic)
	if closeTopic == "" {
		closeTopic = "knowledge.decay.task.close"
	}
	return &eventBusDispatcher{bus: bus, dispatchTopic: dispatchTopic, closeTopic: closeTopic}
}

func (d *eventBusDispatcher) Dispatch(ctx context.Context, task *models.DecayTask) error {
	if d == nil || d.bus == nil || task == nil {
		return nil
	}
	payload := map[string]any{
		"task_id":      task.UUID.String(),
		"space_id":     task.SpaceUUID.String(),
		"category":     task.Category,
		"severity":     task.Severity,
		"status":       task.Status,
		"sla_due_at":   task.SLADueAt.UTC().Format(time.RFC3339Nano),
		"detected_at":  task.DetectedAt.UTC().Format(time.RFC3339Nano),
		"assigned_to":  strings.TrimSpace(task.AssignedTo),
		"description":  strings.TrimSpace(task.Resolution),
		"requires_approval": true,
	}
	d.bus.Publish(d.dispatchTopic, payload, ctx)
	return nil
}

func (d *eventBusDispatcher) Close(ctx context.Context, task *models.DecayTask) error {
	if d == nil || d.bus == nil || task == nil {
		return nil
	}
	payload := map[string]any{
		"task_id":        task.UUID.String(),
		"space_id":       task.SpaceUUID.String(),
		"status":         task.Status,
		"false_positive": task.FalsePositive,
		"resolved_at":    time.Now().UTC().Format(time.RFC3339Nano),
	}
	d.bus.Publish(d.closeTopic, payload, ctx)
	return nil
}

