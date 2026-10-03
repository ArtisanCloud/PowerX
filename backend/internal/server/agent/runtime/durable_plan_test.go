package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestBuildInvokePlanStopsBeforeExecution(t *testing.T) {
	engine := &Engine{}
	plan, err := engine.BuildInvokePlan(context.Background(), "execute this flow", nil, "flow.review")
	require.NoError(t, err)
	require.Len(t, plan.Tasks, 1)
	require.Equal(t, "flow.review", plan.Tasks[0].FlowID)
	require.Equal(t, "default", plan.Tasks[0].TaskID)
	_, err = engine.BuildInvokePlan(context.Background(), "", nil, "flow.review")
	require.Error(t, err)
	_, err = engine.BuildInvokePlan(context.Background(), "execute", nil, "")
	require.Error(t, err)
}

func TestTranslateInvokePlanPreservesParallelStageAndSerialBarrier(t *testing.T) {
	source := &flowschema.ExecutionPlan{PlanID: "plan.test", Tasks: []flowschema.PlanTask{
		{TaskID: "source", Stage: 1, NodeKind: "llm", NodeRef: "qwen"},
		{TaskID: "campaign", Stage: 1, NodeKind: "llm", NodeRef: "qwen"},
		{TaskID: "summary", Stage: 2, NodeKind: "workflow"},
	}}
	pool := func(task flowschema.PlanTask) (string, error) {
		if task.NodeKind == "llm" {
			return "model:qwen", nil
		}
		return "workflow", nil
	}
	plan, err := TranslateInvokePlan(source, pool)
	require.NoError(t, err)
	require.EqualValues(t, 1, plan.Revision)
	require.Empty(t, plan.Tasks[0].DependsOn)
	require.Empty(t, plan.Tasks[1].DependsOn)
	require.ElementsMatch(t, []string{"source", "campaign"}, plan.Tasks[2].DependsOn)
	require.Equal(t, "model:qwen", plan.Tasks[0].PoolID)
	require.True(t, agent_run.ValidPlan(plan))
}

func TestTranslateInvokePlanRejectsInvalidOrUnresolvedTask(t *testing.T) {
	source := &flowschema.ExecutionPlan{Tasks: []flowschema.PlanTask{{TaskID: "a", Stage: 1}}}
	_, err := TranslateInvokePlan(source, func(flowschema.PlanTask) (string, error) { return "", nil })
	require.ErrorIs(t, err, agent_run.ErrInvalid)
	_, err = TranslateInvokePlan(source, func(flowschema.PlanTask) (string, error) { return "", errors.New("pool_missing") })
	require.ErrorIs(t, err, agent_run.ErrInvalid)
	source.Tasks = append(source.Tasks, flowschema.PlanTask{TaskID: "a", Stage: 2})
	_, err = TranslateInvokePlan(source, func(flowschema.PlanTask) (string, error) { return "workflow", nil })
	require.ErrorIs(t, err, agent_run.ErrInvalid)
	source.Tasks[1].TaskID = "b"
	source.Tasks[0].DependsOn = []string{"b"}
	_, err = TranslateInvokePlan(source, func(flowschema.PlanTask) (string, error) { return "workflow", nil })
	require.ErrorIs(t, err, agent_run.ErrInvalid)
}
