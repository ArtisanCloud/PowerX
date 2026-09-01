package skillsintegration

import (
	"context"
	"testing"

	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestSkillAgentA2APartialFailureContinue(t *testing.T) {
	m := agentpkg.NewAgentManager()
	child := &a2aStubAgent{failFlows: map[string]bool{"flow.child.fail": true}}
	parent := &a2aStubAgent{}
	require.NoError(t, m.Register("agent.child", child, &agentschema.AgentMeta{FlowID: "flow.child.ok"}))
	require.NoError(t, m.Register("agent.parent", parent, &agentschema.AgentMeta{FlowID: "flow.final"}))
	require.NoError(t, m.SetDefaultAgent("agent.parent", "flow.final"))
	m.SetAgentHandoffInvoker(newA2AHandoffInvoker(map[string]bool{"flow.child.fail": true}))

	plan := flowschema.ExecutionPlan{
		PlanID: "plan-a2a-partial-fail",
		Tasks: []flowschema.PlanTask{
			{TaskID: "handoff-fail", FlowID: "flow.child.fail", AgentID: "agent.child", NodeKind: "agent_handoff", FailurePolicy: "continue", Stage: 1},
			{TaskID: "handoff-ok", FlowID: "flow.child.ok", AgentID: "agent.child", NodeKind: "agent_handoff", FailurePolicy: "continue", Stage: 1},
			{TaskID: "final", FlowID: "flow.final", AgentID: "agent.parent", NodeKind: "workflow", Stage: 2, DependsOn: []string{"handoff-fail", "handoff-ok"}},
		},
	}

	out, err := m.ExecutePlan(context.Background(), plan, agentschema.ExecutionMeta{RequestID: "req-a2a-partial-fail", TenantUUID: "tenant-a2a"})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.True(t, out.Success)
	require.Equal(t, "flow.final", out.Data["flow_id"])
}
