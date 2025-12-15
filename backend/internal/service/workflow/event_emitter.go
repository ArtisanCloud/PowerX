package workflow

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
)

// eventEmitter 为工作流域写入审计事件提供轻量适配器。
type eventEmitter struct {
	recorder EventRecorder
	clock    func() time.Time
}

func newEventEmitter(recorder EventRecorder, clock func() time.Time) *eventEmitter {
	if clock == nil {
		clock = time.Now
	}
	return &eventEmitter{
		recorder: recorder,
		clock:    clock,
	}
}

func (e *eventEmitter) emit(ctx context.Context, evt *modelworkflow.WorkflowEvent) {
	if e == nil || e.recorder == nil || evt == nil {
		return
	}
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = e.clock().UTC()
	}
	_ = e.recorder.RecordEvent(ctx, evt)
}

func toJSONPayload(payload map[string]any) datatypes.JSON {
	if payload == nil {
		return datatypes.JSON([]byte(`{}`))
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return datatypes.JSON([]byte(`{}`))
	}
	return datatypes.JSON(bytes)
}

func newWorkflowEvent(tenantUUID string, instanceUUID uuid.UUID, eventType string, summary string, payload map[string]any) *modelworkflow.WorkflowEvent {
	return &modelworkflow.WorkflowEvent{
		TenantUUID:   strings.ToLower(strings.TrimSpace(tenantUUID)),
		WorkflowUUID: instanceUUID,
		EventType:    eventType,
		Summary:      summary,
		Payload:      toJSONPayload(payload),
		OccurredAt:   time.Now().UTC(),
	}
}
