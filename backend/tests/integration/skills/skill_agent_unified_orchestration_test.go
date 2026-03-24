package skillsintegration

import (
	"context"
	"testing"

	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/stretchr/testify/require"
)

func TestSkillAgentUnifiedOrchestration_SeededThirdPartySkillWithoutFlowRoute(t *testing.T) {
	m := agentpkg.NewAgentManager()
	m.SetSkillInvoker(func(ctx context.Context, in agentpkg.SkillInvokeInput) (*agentpkg.SkillInvokeOutput, error) {
		return &agentpkg.SkillInvokeOutput{
			TraceID:      "trace-test-skill",
			Status:       "completed",
			ProtocolUsed: "skill",
			SkillID:      in.SkillID,
			Version:      "1.0.0",
			Result: map[string]any{
				"node_kind": "skill",
				"node_ref":  in.SkillID,
			},
		}, nil
	})
	m.UpsertUnifiedCandidate(agentpkg.ToolCallCandidate{
		Name:        "skill.thirdparty.hello-echo",
		NodeKind:    "skill",
		NodeRef:     "skill.thirdparty.hello-echo",
		Description: "echo hello text",
		IntentHints: []string{"hello", "echo"},
		Tags:        []string{"third_party"},
	})

	tasks, err := m.DetectTasks(context.Background(), "请调用 hello echo 技能并返回结果")
	require.NoError(t, err)
	require.NotEmpty(t, tasks)
	require.Equal(t, "skill.thirdparty.hello-echo", tasks[0].FlowID)
	require.Contains(t, tasks[0].Strategy, "skill")

	plan := m.BuildPlan(tasks)
	require.NotEmpty(t, plan.Tasks)
	require.Equal(t, "skill", plan.Tasks[0].NodeKind)
	require.Equal(t, "skill.thirdparty.hello-echo", plan.Tasks[0].NodeRef)

	out, err := m.ExecutePlan(context.Background(), plan, agentschema.ExecutionMeta{RequestID: "req-unified-skill"})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.True(t, out.Success)
	result, ok := out.Data["result"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "skill", result["node_kind"])
	require.Equal(t, "skill.thirdparty.hello-echo", result["node_ref"])
}
