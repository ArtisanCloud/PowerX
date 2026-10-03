package runtime

import (
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPlanControllerFreezesSnapshotAndBudget(t *testing.T) {
	snapshot, err := NewResourceSnapshot(uuid.NewString(), uuid.New(), nil, time.Now())
	require.NoError(t, err)
	controller, err := NewPlanController(snapshot, &RuntimeBudget{MaxPlanRevisions: 1}, flowschema.ExecutionPlan{PlanID: "initial", Tasks: []flowschema.PlanTask{{TaskID: "task_initial"}}})
	require.NoError(t, err)
	revision, err := controller.Revise("verification.replaceable", flowschema.ExecutionPlan{PlanID: "revised", Tasks: []flowschema.PlanTask{{TaskID: "task_replacement"}}})
	require.NoError(t, err)
	require.Equal(t, snapshot.SnapshotUUID, revision.SnapshotUUID)
	require.NotNil(t, revision.ParentRevisionUUID)
	require.Equal(t, []string{"task_initial"}, revision.SupersededTaskRefs)
	require.Equal(t, []string{"task_replacement"}, revision.NewTaskRefs)
	_, err = controller.Revise("verification.retryable", flowschema.ExecutionPlan{PlanID: "second"})
	require.Error(t, err)
}

func TestRuntimeBudgetEnforcesStepsCapabilityCallsAndConcurrency(t *testing.T) {
	budget := RuntimeBudget{MaxSteps: 2, MaxCapabilityCalls: 1, MaxConcurrentTasks: 1}
	require.NoError(t, budget.ConsumeTask("skill"))
	require.NoError(t, budget.ConsumeTask("tooling"))
	require.ErrorIs(t, budget.ConsumeTask("tooling"), ErrRuntimeBudgetExhausted)
	concurrent := flowschema.ExecutionPlan{PlanID: "concurrent", Tasks: []flowschema.PlanTask{{TaskID: "a", Stage: 0}, {TaskID: "b", Stage: 0}}}
	require.NoError(t, budget.ValidatePlanConcurrency(concurrent))
}
