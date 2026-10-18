package system_recovery

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func (d *testDriver) PreflightRestore(context.Context, *snapshots.ManagedJob, Deployment) ([]snapshots.RecoveryCheck, error) {
	return []snapshots.RecoveryCheck{{Name: "preflight", Passed: true}}, nil
}
func (d *testDriver) FinishReplacement(context.Context, snapshots.ManagedJob) error { return nil }

func instanceFixture(t *testing.T) (*Coordinator, *testDriver, snapshots.ManagedJob) {
	t.Helper()
	store, err := snapshots.NewCoordinatorStore(t.TempDir(), []snapshots.RecoveryTarget{{ID: "current", Mode: "formal", Environment: "prod", AllowOverwrite: true}})
	require.NoError(t, err)
	id := uuid.NewString()
	d := &testDriver{active: id, rollbackID: uuid.NewString()}
	c := &Coordinator{Store: store, Driver: d, Initial: Deployment{ID: id, Root: "/owned/instance", ConfigFile: "/owned/instance/etc/config.yaml"}}
	_, err = c.Active()
	require.NoError(t, err)
	j, err := store.EnqueueInstanceRestore(privileged(context.Background()), uuid.NewString(), uuid.NewString(), 1)
	require.NoError(t, err)
	return c, d, j
}
func TestInstanceRestoreKeepsIdentityAndSingleConfig(t *testing.T) {
	c, d, j := instanceFixture(t)
	require.NoError(t, c.RunOnce(context.Background()))
	j, err := c.Store.Get(j.ID)
	require.NoError(t, err)
	require.Equal(t, "complete", j.Status, j.Error)
	require.Equal(t, d.rollbackID, j.RollbackSnapshotID)
	a, err := c.Active()
	require.NoError(t, err)
	require.Equal(t, c.Initial, a.Deployment)
	require.Empty(t, a.LastSwitchID)
	require.False(t, a.Maintenance)
	require.EqualValues(t, 2, a.Generation)
	// The protection snapshot is restored through exactly the same operation.
	back, err := c.Store.EnqueueInstanceRestore(privileged(context.Background()), j.RollbackSnapshotID, uuid.NewString(), a.Generation)
	require.NoError(t, err)
	require.Equal(t, "restore", back.Kind)
	require.Equal(t, "replace_instance", back.RestoreMode)
}
func TestInstanceRestoreFencesConcurrentAndStaleRequests(t *testing.T) {
	c, _, j := instanceFixture(t)
	ctx := privileged(context.Background())
	_, err := c.Store.EnqueueInstanceRestore(ctx, uuid.NewString(), uuid.NewString(), 1)
	require.Error(t, err)
	require.NoError(t, c.RunOnce(ctx))
	_, err = c.Store.EnqueueInstanceRestore(ctx, j.SnapshotID, uuid.NewString(), 1)
	require.Error(t, err)
}

func TestInstanceRestoreWithoutProtectionCannotReplace(t *testing.T) {
	c, d, j := instanceFixture(t)
	d.rollbackID = ""
	var phases []string
	c.Checkpoint = func(p string) error { phases = append(phases, p); return nil }
	require.NoError(t, c.RunOnce(context.Background()))
	j, err := c.Store.Get(j.ID)
	require.NoError(t, err)
	require.Equal(t, "rolled_back", j.Status, j.Error)
	require.NotContains(t, phases, "instance_replaced")
	a, err := c.Active()
	require.NoError(t, err)
	require.Equal(t, c.Initial, a.Deployment)
	require.False(t, a.Maintenance)
}

func TestInstanceRestoreReplayAfterCompletionKeepsReceipt(t *testing.T) {
	c, _, j := instanceFixture(t)
	key := uuid.NewString()
	j.Status = "failed"
	require.NoError(t, c.Store.Save(j))
	first, err := c.Store.EnqueueInstanceRestore(privileged(context.Background()), j.SnapshotID, key, 1)
	require.NoError(t, err)
	require.NoError(t, c.RunOnce(context.Background()))
	replay, err := c.Store.EnqueueInstanceRestore(privileged(context.Background()), j.SnapshotID, key, 1)
	require.NoError(t, err)
	require.Equal(t, first.ID, replay.ID)
}
func TestInstanceRestoreCompensatesBeforeWritesAndHoldsAfterWrites(t *testing.T) {
	for _, phase := range []string{"maintenance", "rollback_point_saved", "instance_replaced", "resuming_writes"} {
		t.Run(phase, func(t *testing.T) {
			c, _, j := instanceFixture(t)
			c.Checkpoint = func(p string) error {
				if p == phase {
					return errors.New("injected crash")
				}
				return nil
			}
			require.NoError(t, c.RunOnce(context.Background()))
			got, err := c.Store.Get(j.ID)
			require.NoError(t, err)
			a, err := c.Active()
			require.NoError(t, err)
			require.Equal(t, c.Initial, a.Deployment)
			if phase == "resuming_writes" {
				require.Equal(t, "attention_required", got.Status)
				require.True(t, a.Maintenance)
			} else {
				require.Equal(t, "rolled_back", got.Status)
				require.False(t, a.Maintenance)
			}
		})
	}
}
func TestInterruptedRestoreDuringPreflightDoesNotEnterMaintenance(t *testing.T) {
	c, _, j := instanceFixture(t)
	j.Status = "running"
	j.Phase = "materializing"
	require.NoError(t, c.Store.Save(j))
	require.NoError(t, c.Recover(context.Background()))
	j, err := c.Store.Get(j.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", j.Status)
	a, err := c.Active()
	require.NoError(t, err)
	require.False(t, a.Maintenance)
}
func TestInterruptedInstanceReplacementCompensatesFromDisk(t *testing.T) {
	c, _, j := instanceFixture(t)
	a, err := c.Active()
	require.NoError(t, err)
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(c.Store.Directory, "previous-"+j.ID+".json"), a))
	j.Status = "running"
	j.Phase = "instance_replaced"
	require.NoError(t, c.Store.Save(j))
	require.NoError(t, c.Recover(context.Background()))
	j, err = c.Store.Get(j.ID)
	require.NoError(t, err)
	require.Equal(t, "rolled_back", j.Status, j.Error)
}
