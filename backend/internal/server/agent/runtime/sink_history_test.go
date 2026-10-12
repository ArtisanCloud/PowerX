package runtime

import (
	"testing"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func TestHistorySinkMergesRunStateTaskUpdatesByTaskID(t *testing.T) {
	history := &HistorySink{}
	history.captureRunStateTask(dto.EventAgentRunTaskStarted, dto.AgentTaskState{
		TaskID:        "campaign_analysis",
		Stage:         2,
		ParallelGroup: "stage_2",
		AgentName:     "Campaign Analyst",
		Status:        dto.AgentTaskStatusRunning,
	})
	history.captureRunStateTask(dto.EventAgentRunTaskCompleted, dto.AgentTaskState{
		TaskID: "campaign_analysis",
		Status: dto.AgentTaskStatusCompleted,
		Result: map[string]any{
			"summary": "done",
		},
	})

	tasks, ok := history.runState["tasks"].([]any)
	if !ok || len(tasks) != 1 {
		t.Fatalf("history task snapshot must contain one merged task, got %#v", history.runState)
	}
	task := mapFromAny(tasks[0])
	if got := task["status"]; got != dto.AgentTaskStatusCompleted {
		t.Fatalf("status=%v want completed; task=%#v", got, task)
	}
	if got := task["stage"]; got != 2 {
		t.Fatalf("stage=%v want 2; task=%#v", got, task)
	}
	if got := task["agent_name"]; got != "Campaign Analyst" {
		t.Fatalf("agent_name=%v want Campaign Analyst; task=%#v", got, task)
	}
}

func TestHistorySinkClearsSupersededPendingTaskAndRecordsEnd(t *testing.T) {
	history := &HistorySink{}
	history.captureRunStateTask(dto.EventAgentRunAwaitingParams, dto.AgentTaskState{
		TaskID: "collect_budget",
		Status: dto.AgentTaskStatusAwaitingParams,
	})
	history.captureRunStateTask(dto.EventAgentRunTaskCompleted, dto.AgentTaskState{
		TaskID: "collect_budget",
		Status: dto.AgentTaskStatusCompleted,
		Result: map[string]any{"budget": 342000},
	})
	history.markRunStateEnded()

	if history.pending != nil {
		t.Fatalf("terminal update must clear pending task, got %#v", history.pending)
	}
	if ended, _ := history.runState["ended"].(bool); !ended {
		t.Fatalf("run end was not recorded: %#v", history.runState)
	}
}

func TestHistorySinkRetainsEnvelopeFromFlowResultFinal(t *testing.T) {
	envelope := map[string]any{"schema": "powerx.agent.response/v4"}
	meta := extractAssistantTraceMeta(dto.AgentRunEvent{
		Event: dto.EventAgentRunFinal,
		Payload: map[string]any{
			"data": flowschema.Result{"response_envelope": envelope},
		},
	})
	if got, ok := meta["response_envelope"].(map[string]any); !ok || got["schema"] != "powerx.agent.response/v4" {
		t.Fatalf("flow result response envelope must be retained for history persistence, got %#v", meta)
	}
}
