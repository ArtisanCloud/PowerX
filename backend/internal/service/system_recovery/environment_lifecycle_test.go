package system_recovery

import (
	"context"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func (d *testDriver) DeleteEnvironment(context.Context, Deployment) error { return nil }
func (d *testDriver) ReplaceCurrent(_ context.Context, _ *snapshots.ManagedJob, current Deployment) (Deployment, []snapshots.RecoveryCheck, error) {
	return current, []snapshots.RecoveryCheck{{Name: "business", Passed: true}}, nil
}
func (d *testDriver) RevertReplacement(context.Context, snapshots.ManagedJob, string) error {
	return nil
}
func TestExplicitReturnAndWholeEnvironmentCleanup(t *testing.T) {
	ctx := privileged(context.Background())
	c, d, cut := coordinatorFixture(t)
	require.NoError(t, c.RunOnce(ctx))
	a, e := c.Active()
	require.NoError(t, e)
	require.Equal(t, cut.ID, a.LastSwitchID)
	back, e := c.Store.EnqueueRollback(ctx, cut.ID, uuid.NewString(), a.Generation)
	require.NoError(t, e)
	require.NoError(t, c.RunOnce(ctx))
	back, e = c.Store.Get(back.ID)
	require.NoError(t, e)
	require.Equal(t, "complete", back.Status, back.Error)
	require.NotEmpty(t, back.RollbackSnapshotID)
	a, e = c.Active()
	require.NoError(t, e)
	require.Equal(t, "source", a.Deployment.ID)
	require.EqualValues(t, 3, a.Generation)
	require.False(t, a.Maintenance)
	require.Equal(t, "source", d.active)
	cleanup, e := c.Store.EnqueueCleanup(ctx, cut.CandidateID, uuid.NewString(), a.Generation)
	require.NoError(t, e)
	require.NoError(t, c.RunOnce(ctx))
	cleanup, e = c.Store.Get(cleanup.ID)
	require.NoError(t, e)
	require.Equal(t, "complete", cleanup.Status, cleanup.Error)
	candidate, e := c.Store.Get(cut.CandidateID)
	require.NoError(t, e)
	require.Equal(t, "deleted", candidate.Status)
}
func TestStaleReturnAndActiveCleanupAreRejected(t *testing.T) {
	ctx := privileged(context.Background())
	c, _, cut := coordinatorFixture(t)
	require.NoError(t, c.RunOnce(ctx))
	stale, e := c.Store.EnqueueRollback(ctx, cut.ID, uuid.NewString(), 1)
	require.NoError(t, e)
	require.NoError(t, c.RunOnce(ctx))
	stale, e = c.Store.Get(stale.ID)
	require.NoError(t, e)
	require.Equal(t, "failed", stale.Status)
	active, e := c.Active()
	require.NoError(t, e)
	require.False(t, active.Maintenance)
	cleanup, e := c.Store.EnqueueCleanup(ctx, cut.CandidateID, uuid.NewString(), active.Generation)
	require.NoError(t, e)
	require.NoError(t, c.RunOnce(ctx))
	cleanup, e = c.Store.Get(cleanup.ID)
	require.NoError(t, e)
	require.Equal(t, "failed", cleanup.Status)
}
func TestProductionRequiresExplicitFixedInstallation(t *testing.T) {
	_, e := snapshots.NewCoordinatorStore(t.TempDir(), []snapshots.RecoveryTarget{{ID: "production", Mode: "formal", Environment: "prod", AllowOverwrite: true}})
	require.NoError(t, e)
	p := Profile{InPlace: true}
	require.Error(t, p.Validate())
}

func TestInterruptedOverwriteBeforeWritesReturnsOriginal(t *testing.T) {
	ctx := privileged(context.Background())
	c, d, j := coordinatorFixture(t)
	j.Kind = "overwrite"
	j.Status = "running"
	j.Phase = "mapping_configuration"
	require.NoError(t, c.Store.Save(j))
	a, e := c.Active()
	require.NoError(t, e)
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(c.Store.Directory, "previous-"+j.ID+".json"), a))
	a.Maintenance = true
	a.JobID = j.ID
	require.NoError(t, c.saveActive(a))
	require.NoError(t, c.Recover(ctx))
	j, e = c.Store.Get(j.ID)
	require.NoError(t, e)
	require.Equal(t, "rolled_back", j.Status, j.Error)
	a, e = c.Active()
	require.NoError(t, e)
	require.False(t, a.Maintenance)
	require.Equal(t, "source", d.active)
}

func TestEnvironmentOverviewProjectsFullPrivateDeployment(t *testing.T) {
	c, _, _ := coordinatorFixture(t)
	a, e := c.Active()
	require.NoError(t, e)
	a.Deployment.Root = "/private/source"
	a.Deployment.ConfigFile = "/private/secret-config"
	a.Deployment.ValidationURL = "http://127.0.0.1:33030"
	a.JobID = uuid.NewString()
	require.NoError(t, c.saveActive(a))
	out, e := c.Store.EnvironmentState()
	require.NoError(t, e)
	require.Equal(t, "source", out["active_environment_id"])
	require.NotContains(t, out, "deployment")
	require.NotContains(t, out, "root")
	require.NotContains(t, out, "config_file")
}
