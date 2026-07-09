package runtime

import (
	"context"
	"testing"

	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type captureSink struct {
	events []string
	data   []any
}

func (s *captureSink) Emit(event string, payload any) error {
	s.events = append(s.events, event)
	s.data = append(s.data, payload)
	return nil
}

func TestRunStateSinkTranslatesNodeEvents(t *testing.T) {
	base := &captureSink{}
	ctx := context.WithValue(context.Background(), "session_id", "s1")
	ctx = context.WithValue(ctx, "message_id", "m1")
	sink := NewRunStateSink(ctx, base)

	if err := sink.Emit(dto.EventNodeStart, map[string]any{
		"task_id":   "t1",
		"node_kind": dto.NodeKindSkill,
		"node_ref":  "skill.template",
	}); err != nil {
		t.Fatalf("emit start: %v", err)
	}
	if err := sink.Emit(dto.EventNodeEnd, map[string]any{
		"task_id":   "t1",
		"node_kind": dto.NodeKindSkill,
		"node_ref":  "skill.template",
		"status":    dto.AgentTaskStatusFailed,
		"error":     "boom",
	}); err != nil {
		t.Fatalf("emit end: %v", err)
	}

	want := []string{
		dto.EventAgentRunTaskStarted,
		dto.EventAgentRunTaskFailed,
	}
	if len(base.events) != len(want) {
		t.Fatalf("events=%v", base.events)
	}
	for i := range want {
		if base.events[i] != want[i] {
			t.Fatalf("event[%d]=%s want %s; all=%v", i, base.events[i], want[i], base.events)
		}
	}
	state, ok := base.data[0].(dto.AgentTaskState)
	if !ok {
		t.Fatalf("first payload type=%T", base.data[0])
	}
	if state.Status != dto.AgentTaskStatusRunning || state.SkillID != "skill.template" || state.SessionID != "s1" || state.MessageID != "m1" {
		t.Fatalf("bad mirrored state: %#v", state)
	}
}

func TestRunStateSinkDoesNotCompleteTaskWithoutResultEvidence(t *testing.T) {
	base := &captureSink{}
	sink := NewRunStateSink(context.Background(), base)

	if err := sink.Emit(dto.EventNodeEnd, map[string]any{
		"task_id":   "t1",
		"node_kind": dto.NodeKindSkill,
		"node_ref":  "skill.template",
		"status":    dto.AgentTaskStatusCompleted,
	}); err != nil {
		t.Fatalf("emit end: %v", err)
	}

	if len(base.events) != 1 {
		t.Fatalf("events=%v", base.events)
	}
	if base.events[0] != dto.EventAgentRunTaskStatus {
		t.Fatalf("event=%s want %s", base.events[0], dto.EventAgentRunTaskStatus)
	}
	state, ok := base.data[0].(dto.AgentTaskState)
	if !ok {
		t.Fatalf("payload type=%T", base.data[0])
	}
	if state.Status == dto.AgentTaskStatusCompleted {
		t.Fatalf("task completed without result evidence: %#v", state)
	}
}

func TestRunStateSinkCompletesTaskWithResultEvidence(t *testing.T) {
	base := &captureSink{}
	sink := NewRunStateSink(context.Background(), base)

	if err := sink.Emit(dto.EventNodeEnd, map[string]any{
		"task_id": "t1",
		"status":  dto.AgentTaskStatusCompleted,
		"result_summary": map[string]any{
			"success": true,
		},
	}); err != nil {
		t.Fatalf("emit end: %v", err)
	}

	if len(base.events) != 1 || base.events[0] != dto.EventAgentRunTaskCompleted {
		t.Fatalf("events=%v", base.events)
	}
}

func TestExtractAssistantTextFromAgentRunFinal(t *testing.T) {
	payload := dto.AgentRunEvent{
		Event: dto.EventFinal,
		Payload: map[string]any{
			"success": true,
			"data": map[string]any{
				"content": "模板智能体可以帮你管理模板对象。",
			},
		},
	}

	if got := extractAssistantText(payload); got != "模板智能体可以帮你管理模板对象。" {
		t.Fatalf("extractAssistantText()=%q", got)
	}
}

func TestEnrichTaskEndPayloadIncludesSkillBusinessResult(t *testing.T) {
	payload := map[string]any{}
	enrichTaskEndPayload(payload, &agentschema.ExecutionResult{
		Success: true,
		Data: map[string]any{
			"result": map[string]any{
				"content": "已创建模板「合同模板」。",
				"links": []map[string]any{
					{"label": "查看模板", "href": "/templates/crud?template_id=12"},
				},
			},
		},
	})

	if payload["message"] != "已创建模板「合同模板」。" {
		t.Fatalf("message=%v", payload["message"])
	}
	if _, ok := payload["result"].(map[string]any); !ok {
		t.Fatalf("missing result: %#v", payload)
	}
	links, ok := payload["links"].([]map[string]any)
	if !ok || len(links) != 1 || links[0]["href"] != "/templates/crud?template_id=12" {
		t.Fatalf("bad links: %#v", payload["links"])
	}
}
