package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type RunStateSink struct {
	next     EventSink
	ctx      context.Context
	recorder func(event string, payload any)
}

func NewRunStateSink(ctx context.Context, next EventSink) EventSink {
	if next == nil {
		return nil
	}
	return &RunStateSink{next: next, ctx: ctx}
}

func (s *RunStateSink) Emit(event string, payload any) error {
	if s == nil || s.next == nil {
		return nil
	}
	if strings.HasPrefix(event, "agent_run.") {
		s.record(event, payload)
		return s.next.Emit(event, payload)
	}
	if translatedEvent, translatedPayload, ok := s.translate(event, payload); ok {
		s.record(translatedEvent, translatedPayload)
		return s.next.Emit(translatedEvent, translatedPayload)
	}
	return s.next.Emit(event, payload)
}

func (s *RunStateSink) SetRunStateRecorder(recorder func(event string, payload any)) {
	if s == nil {
		return
	}
	s.recorder = recorder
}

func (s *RunStateSink) record(event string, payload any) {
	if s == nil || s.recorder == nil {
		return
	}
	s.recorder(event, payload)
}

func (s *RunStateSink) translate(event string, payload any) (string, any, bool) {
	switch event {
	case dto.EventStart:
		return dto.EventAgentRunStarted, s.wrap(event, payload), true
	case "response_plan":
		return dto.EventAgentRunResponsePlan, s.wrap(event, payload), true
	case dto.EventIntent:
		return dto.EventAgentRunIntentDetected, s.wrap(event, payload), true
	case dto.EventPlan:
		return dto.EventAgentRunPlanCreated, s.planState(event, payload), true
	case dto.EventNodeStart:
		return dto.EventAgentRunTaskStarted, s.taskState(payload, dto.AgentTaskStatusRunning), true
	case dto.EventNodeEnd:
		rawStatus := strings.ToLower(strings.TrimSpace(stringFromAny(mapValue(payload, "status"))))
		status := dto.AgentTaskStatusPending
		if strings.EqualFold(rawStatus, dto.AgentTaskStatusFailed) || taskPayloadIndicatesFailure(payload) {
			status = dto.AgentTaskStatusFailed
		} else if strings.EqualFold(rawStatus, dto.AgentTaskStatusCompleted) && hasTaskCompletionEvidence(payload) {
			status = dto.AgentTaskStatusCompleted
		}
		if status == dto.AgentTaskStatusFailed {
			return dto.EventAgentRunTaskFailed, s.taskState(payload, status), true
		}
		if status == dto.AgentTaskStatusCompleted {
			return dto.EventAgentRunTaskCompleted, s.taskState(payload, status), true
		}
		return dto.EventAgentRunTaskStatus, s.taskState(payload, status), true
	case dto.EventFinal:
		return dto.EventAgentRunFinal, s.wrap(event, payload), true
	case dto.EventError:
		return dto.EventAgentRunTaskFailed, s.taskState(payload, dto.AgentTaskStatusFailed), true
	case dto.EventEnd:
		return dto.EventAgentRunEnded, s.wrap(event, payload), true
	default:
		return "", nil, false
	}
}

func taskPayloadIndicatesFailure(payload any) bool {
	if strings.TrimSpace(stringFromAny(mapValue(payload, "error"))) != "" {
		return true
	}
	resultSummary, ok := mapValue(payload, "result_summary").(map[string]any)
	if !ok {
		return false
	}
	success, exists := resultSummary["success"]
	value, isBoolean := success.(bool)
	return exists && isBoolean && !value
}

func hasTaskCompletionEvidence(payload any) bool {
	for _, key := range []string{"result", "result_summary", "data", "links"} {
		value := mapValue(payload, key)
		if value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return true
			}
		case []any:
			if len(v) > 0 {
				return true
			}
		case []map[string]any:
			if len(v) > 0 {
				return true
			}
		case map[string]any:
			if len(v) > 0 {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func (s *RunStateSink) wrap(event string, payload any) dto.AgentRunEvent {
	return dto.AgentRunEvent{
		RunID:     s.ctxString("run_id", "runId"),
		SessionID: s.ctxString("session_id", "sessionId"),
		MessageID: s.ctxString("message_id", "messageId"),
		TraceID:   s.traceID(),
		Event:     event,
		Payload:   payload,
	}
}

func (s *RunStateSink) planState(event string, payload any) dto.AgentRunEvent {
	wrapped := s.wrap(event, payload)
	tasks := planTasksFromPayload(payload)
	if len(tasks) == 0 {
		return wrapped
	}
	wrapped.Payload = map[string]any{
		"event":   event,
		"payload": payload,
		"summary": dto.AgentRunSummary{
			RunID:        wrapped.RunID,
			SessionID:    wrapped.SessionID,
			MessageID:    wrapped.MessageID,
			TraceID:      wrapped.TraceID,
			Status:       dto.AgentTaskStatusPending,
			TotalTasks:   len(tasks),
			PendingTasks: len(tasks),
			TotalStages:  maxTaskStage(tasks),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		},
		"tasks": tasks,
	}
	return wrapped
}

func (s *RunStateSink) taskState(payload any, status string) dto.AgentTaskState {
	if status == "" {
		status = dto.AgentTaskStatusPending
	}
	nodeKind := stringFromAny(mapValue(payload, "node_kind"))
	nodeRef := stringFromAny(mapValue(payload, "node_ref"))
	state := dto.AgentTaskState{
		RunID:         s.ctxString("run_id", "runId"),
		SessionID:     s.ctxString("session_id", "sessionId"),
		MessageID:     s.ctxString("message_id", "messageId"),
		TraceID:       s.traceID(),
		TaskID:        firstNonEmpty(stringFromAny(mapValue(payload, "task_id")), stringFromAny(mapValue(payload, "node_id"))),
		ParentTaskID:  stringFromAny(mapValue(payload, "parent_task_id")),
		DependsOn:     stringSliceFromAny(mapValue(payload, "depends_on")),
		Stage:         intFromAny(mapValue(payload, "stage")),
		ParallelGroup: stringFromAny(mapValue(payload, "parallel_group")),
		TeamID:        stringFromAny(mapValue(payload, "team_id")),
		AgentID:       firstNonEmpty(stringFromAny(mapValue(payload, "agent_id")), s.ctxString("agent_id", "agentId")),
		AgentKey:      stringFromAny(mapValue(payload, "agent_key")),
		AgentName:     stringFromAny(mapValue(payload, "agent_name")),
		NodeKind:      nodeKind,
		NodeRef:       nodeRef,
		SkillID:       skillIDFromNode(nodeKind, nodeRef),
		CapabilityID:  stringFromAny(mapValue(payload, "capability_id")),
		Status:        status,
		Action:        actionFromPayload(payload),
		FailurePolicy: stringFromAny(mapValue(payload, "failure_policy")),
		Message:       firstNonEmpty(stringFromAny(mapValue(payload, "message")), stringFromAny(mapValue(payload, "display_message"))),
		Summary:       firstNonEmpty(stringFromAny(mapValue(payload, "summary")), stringFromAny(mapValue(payload, "result_message"))),
		Result:        firstPresent(mapValue(payload, "result"), mapValue(payload, "result_summary"), mapValue(payload, "data")),
		Error:         taskErrorFromPayload(payload, status),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	if state.TaskID == "" {
		state.TaskID = fmt.Sprintf("task_%d", time.Now().UnixNano())
	}
	if fields := stringSliceFromAny(mapValue(payload, "missing_fields")); len(fields) > 0 {
		state.MissingFields = fields
		state.Status = dto.AgentTaskStatusAwaitingParams
	}
	if params, ok := mapValue(payload, "collected_params").(map[string]any); ok {
		state.CollectedParams = params
	}
	if links, ok := mapValue(payload, "links").([]map[string]any); ok {
		state.Links = links
	}
	return state
}

func taskErrorFromPayload(payload any, status string) any {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case dto.AgentTaskStatusFailed, "canceled", "cancelled":
		return firstPresent(mapValue(payload, "error"), mapValue(payload, "detail"), mapValue(payload, "message"))
	default:
		return firstPresent(mapValue(payload, "error"), mapValue(payload, "detail"))
	}
}

func planTasksFromPayload(payload any) []dto.AgentTaskState {
	planPayload := mapValue(payload, "plan")
	if planPayload == nil {
		planPayload = payload
	}
	rawTasks := mapValue(planPayload, "tasks")
	items, ok := rawTasks.([]any)
	if !ok {
		return nil
	}
	tasks := make([]dto.AgentTaskState, 0, len(items))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		nodeKind := stringFromAny(m["node_kind"])
		nodeRef := stringFromAny(m["node_ref"])
		stage := intFromAny(m["stage"])
		parallelGroup := stringFromAny(m["parallel_group"])
		if parallelGroup == "" && stage > 0 {
			parallelGroup = fmt.Sprintf("stage_%d", stage)
		}
		tasks = append(tasks, dto.AgentTaskState{
			TaskID:        stringFromAny(m["task_id"]),
			DependsOn:     stringSliceFromAny(m["depends_on"]),
			Stage:         stage,
			ParallelGroup: parallelGroup,
			TeamID:        stringFromAny(m["team_id"]),
			AgentID:       stringFromAny(m["agent_id"]),
			AgentKey:      stringFromAny(m["agent_key"]),
			AgentName:     stringFromAny(m["agent_name"]),
			NodeKind:      nodeKind,
			NodeRef:       nodeRef,
			SkillID:       skillIDFromNode(nodeKind, nodeRef),
			CapabilityID:  stringFromAny(m["capability_id"]),
			FailurePolicy: stringFromAny(m["failure_policy"]),
			Message:       firstNonEmpty(stringFromAny(m["message"]), stringFromAny(m["display_message"])),
			Summary:       firstNonEmpty(stringFromAny(m["summary"]), stringFromAny(m["result_message"])),
			Status:        dto.AgentTaskStatusPending,
			UpdatedAt:     now,
		})
	}
	return tasks
}

func maxTaskStage(tasks []dto.AgentTaskState) int {
	maxStage := 0
	for _, task := range tasks {
		if task.Stage > maxStage {
			maxStage = task.Stage
		}
	}
	return maxStage
}

func (s *RunStateSink) ctxString(keys ...string) string {
	if s == nil || s.ctx == nil {
		return ""
	}
	for _, key := range keys {
		if v := s.ctx.Value(key); v != nil {
			if out := strings.TrimSpace(fmt.Sprint(v)); out != "" {
				return out
			}
		}
	}
	return ""
}

func (s *RunStateSink) traceID() string {
	if traceID := strings.TrimSpace(reqctx.GetTraceID(s.ctx)); traceID != "" {
		return traceID
	}
	return s.ctxString("trace_id", "traceId")
}

func mapValue(payload any, key string) any {
	if payload == nil {
		return nil
	}
	if m, ok := payload.(map[string]any); ok {
		return m[key]
	}
	return nil
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPresent(values ...any) any {
	for _, value := range values {
		if value == nil {
			continue
		}
		if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		return value
	}
	return nil
}

func skillIDFromNode(kind, ref string) string {
	if strings.EqualFold(strings.TrimSpace(kind), dto.NodeKindSkill) {
		return strings.TrimSpace(ref)
	}
	return ""
}

func actionFromPayload(payload any) string {
	if params, ok := mapValue(payload, "params").(map[string]any); ok {
		for _, key := range []string{"action", "operation", "op"} {
			if value := stringFromAny(params[key]); value != "" {
				return value
			}
		}
	}
	return stringFromAny(mapValue(payload, "action"))
}

func stringSliceFromAny(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		if s := stringFromAny(v); s != "" {
			var n int
			_, _ = fmt.Sscanf(s, "%d", &n)
			return n
		}
		return 0
	}
}
