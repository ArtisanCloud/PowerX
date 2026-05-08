package skillsintegration

import (
	"context"
	"errors"
	"testing"

	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestSkillAgentA2AContextRefAuthzBlocked(t *testing.T) {
	m := agentpkg.NewAgentManager()
	m.SetContextRefAuthorizer(func(ctx context.Context, tenantUUID string, childAgentID uint64, contextRefID string) error {
		return errors.New("agent.context_ref_forbidden")
	})
	child := &a2aStubAgent{}
	require.NoError(t, m.Register("agent.child", child, &agentschema.AgentMeta{FlowID: "flow.child"}))

	plan := flowschema.ExecutionPlan{
		PlanID: "plan-a2a-authz",
		Tasks: []flowschema.PlanTask{
			{TaskID: "handoff-authz", FlowID: "flow.child", AgentID: "agent.child", NodeKind: "agent_handoff", ContextRefID: "ctx-ref-1", FailurePolicy: "fail-fast", Stage: 1},
		},
	}

	out, err := m.ExecutePlan(context.Background(), plan, agentschema.ExecutionMeta{RequestID: "req-a2a-authz", TenantUUID: "tenant-a2a"})
	require.Error(t, err)
	require.Nil(t, out)
	require.Contains(t, err.Error(), "context_ref")
}
