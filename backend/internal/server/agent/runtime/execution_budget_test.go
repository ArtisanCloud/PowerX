package runtime

import (
	"testing"
	"time"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestRunDeadlineDoesNotChangeWithParallelTaskCount(t *testing.T) {
	plan := flowschema.ExecutionPlan{Tasks: []flowschema.PlanTask{
		{TaskID: "a", Stage: 1},
		{TaskID: "b", Stage: 1},
		{TaskID: "c", Stage: 2},
		{TaskID: "d", Stage: 3},
	}}
	duration, err := runDurationForPlan(plan, 30*time.Minute, 5*time.Minute)
	require.NoError(t, err)
	require.Equal(t, 30*time.Minute, duration)

	plan.Tasks = plan.Tasks[:1]
	duration, err = runDurationForPlan(plan, 30*time.Minute, 5*time.Minute)
	require.NoError(t, err)
	require.Equal(t, 30*time.Minute, duration)
}
