package agent

import (
	"context"
	"errors"
	"testing"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestExecutePlanTaskDoesNotRetrySideEffectsInsideWorker(t *testing.T) {
	m := NewAgentManager()
	calls := 0
	m.SetSkillInvoker(func(context.Context, SkillInvokeInput) (*SkillInvokeOutput, error) {
		calls++
		return nil, errors.New("unknown business outcome")
	})
	_, err := m.ExecutePlanTask(context.Background(), flowschema.ExecutionPlan{PlanID: "guarded", Tasks: []flowschema.PlanTask{
		{TaskID: "write", NodeKind: "skill", NodeRef: "skill.write", Stage: 1, FailurePolicy: "retry-once"},
	}}, "write", nil, aschema.ExecutionMeta{})
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func TestExecutePlanTaskUsesPersistedUpstreamAndRunsOnlySelectedTask(t *testing.T) {
	m := NewAgentManager()
	calls := 0
	m.SetSkillInvoker(func(_ context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		calls++
		require.Equal(t, "skill.final", in.SkillID)
		require.Equal(t, "source text", in.Payload["source_text"])
		return &SkillInvokeOutput{Status: "completed", ProtocolUsed: "skill", SkillID: in.SkillID,
			Result: map[string]any{"content": "final text"}}, nil
	})
	plan := flowschema.ExecutionPlan{PlanID: "persisted", Tasks: []flowschema.PlanTask{
		{TaskID: "source", NodeKind: "skill", NodeRef: "skill.source", FlowID: "source", Stage: 1},
		{TaskID: "sibling", NodeKind: "skill", NodeRef: "skill.sibling", FlowID: "sibling", Stage: 1},
		{TaskID: "final", NodeKind: "skill", NodeRef: "skill.final", FlowID: "final", Stage: 2,
			DependsOn: []string{"source"}, ParamRefs: map[string]string{"source_text": "{{task.source.output.result.text}}"}},
	}}
	prior := map[string]*aschema.ExecutionResult{"source": {Success: true,
		Data: flowschema.Result{"result": map[string]any{"text": "source text"}}}}
	out, err := m.ExecutePlanTask(context.Background(), plan, "final", prior,
		aschema.ExecutionMeta{TenantUUID: "tenant", Metadata: map[string]any{"env": "dev"}})
	require.NoError(t, err)
	require.True(t, out.Success)
	require.Equal(t, "final", out.Metadata["task_id"])
	require.Equal(t, 1, calls)
}

func TestExecutePlanTaskRejectsMissingOrUnrelatedPriorResult(t *testing.T) {
	m := NewAgentManager()
	plan := flowschema.ExecutionPlan{PlanID: "persisted", Tasks: []flowschema.PlanTask{
		{TaskID: "source", Stage: 1}, {TaskID: "target", Stage: 2, DependsOn: []string{"source"}},
		{TaskID: "future", Stage: 3},
	}}
	_, err := m.ExecutePlanTask(context.Background(), plan, "target", nil, aschema.ExecutionMeta{})
	require.ErrorContains(t, err, "no successful receipt")
	prior := map[string]*aschema.ExecutionResult{"source": {Success: true}, "future": {Success: true}}
	_, err = m.ExecutePlanTask(context.Background(), plan, "target", prior, aschema.ExecutionMeta{})
	require.ErrorContains(t, err, "invalid prior task result")
}
