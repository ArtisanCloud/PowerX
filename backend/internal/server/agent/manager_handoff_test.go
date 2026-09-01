package agent

import (
	"context"
	"testing"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestExecuteAgentHandoffRequiresDatabaseBackedInvoker(t *testing.T) {
	manager := NewAgentManager()
	task := flowschema.PlanTask{
		TaskID:   "handoff-1",
		FlowID:   "release.knowledge",
		AgentID:  "release.knowledge_analyst",
		NodeKind: "agent_handoff",
		Params: map[string]any{
			"child_agent_id": uint64(11),
		},
	}

	out, err := manager.executeAgentHandoffTask(
		context.Background(),
		task,
		flowschema.Context{"child_agent_id": uint64(11)},
		aschema.ExecutionMeta{TenantUUID: "tenant-001"},
		"release.knowledge_analyst",
	)

	require.Nil(t, out)
	require.EqualError(t, err, "agent.handoff_invoker_unavailable")
}
