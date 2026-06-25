package skillsintegration

import (
	"context"
	"testing"

	agentruntime "github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

type runStateCaptureSink struct {
	events  []string
	payload []any
}

func (s *runStateCaptureSink) Emit(event string, payload any) error {
	s.events = append(s.events, event)
	s.payload = append(s.payload, payload)
	return nil
}

func TestSkillAgentRunStateMapsA2ATasks(t *testing.T) {
	base := &runStateCaptureSink{}
	ctx := context.WithValue(context.Background(), "session_id", "session-a2a")
	ctx = context.WithValue(ctx, "message_id", "message-a2a")
	sink := agentruntime.NewRunStateSink(ctx, base)

	require.NoError(t, sink.Emit(dto.EventNodeStart, map[string]any{
		"task_id":         "knowledge_analysis",
		"node_kind":       dto.NodeKindHandoff,
		"node_ref":        "release.knowledge_analyst",
		"team_id":         "team-release",
		"agent_id":        "agent-knowledge",
		"handoff_task_id": "knowledge_analysis",
	}))
	require.NoError(t, sink.Emit(dto.EventNodeEnd, map[string]any{
		"task_id":         "knowledge_analysis",
		"node_kind":       dto.NodeKindHandoff,
		"node_ref":        "release.knowledge_analyst",
		"team_id":         "team-release",
		"agent_id":        "agent-knowledge",
		"handoff_task_id": "knowledge_analysis",
		"status":          dto.AgentTaskStatusCompleted,
		"result_summary":  map[string]any{"success": true},
	}))
	require.NoError(t, sink.Emit(dto.EventNodeStart, map[string]any{
		"task_id":         "notification_schedule",
		"node_kind":       dto.NodeKindHandoff,
		"node_ref":        "release.notification_planner",
		"team_id":         "team-release",
		"agent_id":        "agent-notify",
		"handoff_task_id": "notification_schedule",
	}))
	require.NoError(t, sink.Emit(dto.EventNodeEnd, map[string]any{
		"task_id":         "notification_schedule",
		"node_kind":       dto.NodeKindHandoff,
		"node_ref":        "release.notification_planner",
		"team_id":         "team-release",
		"agent_id":        "agent-notify",
		"handoff_task_id": "notification_schedule",
		"status":          dto.AgentTaskStatusFailed,
		"error":           "child agent failed",
	}))

	require.Contains(t, base.events, dto.EventAgentRunTaskStarted)
	require.Contains(t, base.events, dto.EventAgentRunTaskCompleted)
	require.Contains(t, base.events, dto.EventAgentRunTaskFailed)

	var failed dto.AgentTaskState
	for i, event := range base.events {
		if event != dto.EventAgentRunTaskFailed {
			continue
		}
		state, ok := base.payload[i].(dto.AgentTaskState)
		require.True(t, ok)
		failed = state
		break
	}
	require.Equal(t, "notification_schedule", failed.TaskID)
	require.Equal(t, dto.NodeKindHandoff, failed.NodeKind)
	require.Equal(t, "team-release", failed.TeamID)
	require.Equal(t, "agent-notify", failed.AgentID)
	require.Equal(t, dto.AgentTaskStatusFailed, failed.Status)
	require.NotEmpty(t, failed.Error)
}
