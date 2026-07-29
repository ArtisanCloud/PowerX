package workflow

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type runtimeEventPublisher func(ctx context.Context, tenantUUID string, instanceUUID uuid.UUID, eventType string, stepID string, details map[string]any)

func (s *Service) publishRuntimeEvent(ctx context.Context, tenantUUID string, instanceUUID uuid.UUID, eventType string, stepID string, details map[string]any) {
	if s == nil || s.instances == nil || s.steps == nil {
		return
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" || instanceUUID == uuid.Nil {
		return
	}
	instance, err := s.instances.GetByUUID(ctx, tenantUUID, instanceUUID)
	if err != nil || instance == nil {
		return
	}
	steps, err := s.steps.ListByInstance(ctx, instanceUUID)
	if err != nil {
		steps = nil
	}
	payload := workflowRuntimeEventPayload(eventType, stepID, instance, steps, details)
	wsbus.DefaultHub.PublishWithContext(ctx, tenantUUID, eventbus.TopicWorkflowInstance, payload, reqctx.GetTraceID(ctx))
}

func workflowRuntimeEventPayload(eventType string, stepID string, instance *modelworkflow.WorkflowInstance, steps []modelworkflow.WorkflowStepRecord, details map[string]any) map[string]any {
	payload := map[string]any{
		"event_type":  strings.TrimSpace(eventType),
		"step_id":     strings.TrimSpace(stepID),
		"occurred_at": time.Now().UTC(),
		"instance":    workflowRuntimeInstancePayload(instance, steps),
	}
	if details != nil {
		payload["details"] = details
	}
	return payload
}

func workflowRuntimeInstancePayload(instance *modelworkflow.WorkflowInstance, steps []modelworkflow.WorkflowStepRecord) map[string]any {
	if instance == nil {
		return nil
	}
	out := map[string]any{
		"uuid":                instance.UUID.String(),
		"tenant_uuid":         instance.TenantUUID,
		"definition_uuid":     instance.DefinitionUUID.String(),
		"definition_version":  instance.DefinitionVersion,
		"state":               instance.State,
		"current_step_id":     instance.CurrentStepID,
		"last_error":          instance.LastError,
		"trace_id":            instance.TraceID,
		"started_at":          instance.StartedAt,
		"completed_at":        instance.CompletedAt,
		"last_transition_at":  instance.LastTransitionAt,
		"input_context":       runtimeJSONToInterface(instance.InputContext),
		"runtime_context":     runtimeJSONToInterface(instance.RuntimeContext),
		"output_context":      runtimeJSONToInterface(instance.OutputContext),
		"tags":                runtimeJSONToStringMap(instance.Tags),
		"agent_uuid":          instance.AgentUUID,
		"initiator_user_uuid": instance.InitiatorUserUUID,
		"next_heartbeat_due":  instance.NextHeartbeatDue,
	}
	if len(steps) == 0 {
		return out
	}
	stepItems := make([]map[string]any, 0, len(steps))
	for i := range steps {
		step := steps[i]
		stepItems = append(stepItems, map[string]any{
			"id":              step.ID,
			"step_id":         step.StepID,
			"type":            step.Type,
			"node_kind":       step.NodeKind,
			"node_ref":        step.NodeRef,
			"state":           step.State,
			"subject_type":    step.SubjectType,
			"subject_uuid":    step.SubjectUUID,
			"tool_grant_id":   step.ToolGrantID,
			"tool_grant_ver":  step.ToolGrantVer,
			"attempt":         step.Attempt,
			"input_mapping":   runtimeJSONToInterface(step.InputMapping),
			"output_mapping":  runtimeJSONToInterface(step.OutputMapping),
			"payload_in":      runtimeJSONToInterface(step.PayloadIn),
			"payload_out":     runtimeJSONToInterface(step.PayloadOut),
			"failure_reason":  step.FailureReason,
			"error_code":      step.ErrorCode,
			"error_message":   step.ErrorMessage,
			"scheduled_at":    step.ScheduledAt,
			"started_at":      step.StartedAt,
			"completed_at":    step.CompletedAt,
			"last_transition": step.LastTransition,
			"awaiting_human":  step.AwaitingHuman,
		})
	}
	out["steps"] = stepItems
	return out
}

func runtimeJSONToInterface(data datatypes.JSON) any {
	if len(data) == 0 {
		return map[string]any{}
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return map[string]any{}
	}
	return value
}

func runtimeJSONToStringMap(data datatypes.JSON) map[string]string {
	if len(data) == 0 {
		return map[string]string{}
	}
	var value map[string]string
	if err := json.Unmarshal(data, &value); err != nil {
		return map[string]string{}
	}
	return value
}
