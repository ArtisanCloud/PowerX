package skillsintegration

import (
	"context"
	"testing"

	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestSkillAgentA2ABasicParallel(t *testing.T) {
	m := agentpkg.NewAgentManager()
	child := &a2aStubAgent{}
	parent := &a2aStubAgent{}
	require.NoError(t, m.Register("agent.child", child, &agentschema.AgentMeta{FlowID: "flow.child"}))
	require.NoError(t, m.Register("agent.parent", parent, &agentschema.AgentMeta{FlowID: "flow.final"}))
	require.NoError(t, m.SetDefaultAgent("agent.parent", "flow.final"))
	m.SetAgentHandoffInvoker(newA2AHandoffInvoker(nil))

	plan := flowschema.ExecutionPlan{
		PlanID: "plan-a2a-basic",
		Tasks: []flowschema.PlanTask{
			{TaskID: "handoff-a", FlowID: "flow.child", AgentID: "agent.child", NodeKind: "agent_handoff", TeamID: "team-1", HandoffTaskID: "task-a", FailurePolicy: "continue", Stage: 1},
			{TaskID: "handoff-b", FlowID: "flow.child", AgentID: "agent.child", NodeKind: "agent_handoff", TeamID: "team-1", HandoffTaskID: "task-b", FailurePolicy: "continue", Stage: 1},
			{TaskID: "final", FlowID: "flow.final", AgentID: "agent.parent", NodeKind: "workflow", Stage: 2, DependsOn: []string{"handoff-a", "handoff-b"}},
		},
	}

	out, err := m.ExecutePlan(context.Background(), plan, agentschema.ExecutionMeta{RequestID: "req-a2a-basic", TenantUUID: "tenant-a2a"})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.True(t, out.Success)
	require.Equal(t, "flow.final", out.Data["flow_id"])
}
