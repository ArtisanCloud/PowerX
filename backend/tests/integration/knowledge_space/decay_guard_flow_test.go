package knowledge_space_integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDecayGuardFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	svc := env.Deps.KnowledgeSpace.DecayGuard
	require.NotNil(t, svc)

	tpl := env.SeedPolicyTemplate("decay-int", "v1")
	space := env.CreateSpaceFixture("decay-int-space", tpl)
	ctx := context.Background()

	tasks, err := svc.RunScan(ctx, space.UUID, 3)
	require.NoError(t, err)
	require.Len(t, tasks, 3)

	for _, task := range tasks {
		require.Equal(t, space.UUID, task.SpaceUUID)
		require.Equal(t, "open", task.Status)
		require.InDelta(t, 7*24, task.SLADueAt.Sub(task.DetectedAt).Hours(), 0.001)
	}

	openList, err := svc.ListOpen(ctx, space.UUID)
	require.NoError(t, err)
	require.Len(t, openList, 3)

	restored, err := svc.Restore(ctx, tasks[0].UUID, "auto remediation", true)
	require.NoError(t, err)
	require.Equal(t, "closed", restored.Status)
	require.NotNil(t, restored.ResolvedAt)
	require.True(t, restored.FalsePositive)
	require.LessOrEqual(t, restored.ResolvedAt.Sub(restored.DetectedAt), 10*time.Minute)

	remaining, err := svc.ListOpen(ctx, space.UUID)
	require.NoError(t, err)
	require.Len(t, remaining, 2)

	_, err = svc.RunScan(ctx, uuid.New(), 0)
	require.Error(t, err)

	snapshotData, err := os.ReadFile(env.DecayReportPath)
	require.NoError(t, err)
	var snapshot struct {
		FalsePositive int `json:"falsePositive"`
	}
	require.NoError(t, json.Unmarshal(snapshotData, &snapshot))
	require.Equal(t, 1, snapshot.FalsePositive)

	aggregateData, err := os.ReadFile(env.KnowledgeUpdateReportPath)
	require.NoError(t, err)
	var state map[string]any
	require.NoError(t, json.Unmarshal(aggregateData, &state))
	require.Contains(t, state, "decay")
}
