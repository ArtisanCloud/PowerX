package system_recovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCurrentSnapshotDeletionCleansOnlyFinishedAssociatedWork(t *testing.T) {
	r, _, _ := currentFixture(t)
	id, other := uuid.NewString(), uuid.NewString()
	job := snapshots.ManagedJob{ID: uuid.NewString(), SnapshotID: id, Status: "failed", Kind: "restore"}
	unrelated := snapshots.ManagedJob{ID: uuid.NewString(), SnapshotID: other, Status: "complete", Kind: "restore"}
	for _, row := range []snapshots.ManagedJob{job, unrelated} {
		require.NoError(t, r.Coordinator.Store.Save(row))
		require.NoError(t, os.MkdirAll(r.work(row.ID), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(r.work(row.ID), "database.dump"), []byte("staged private data"), 0600))
	}
	liveConfig, err := os.ReadFile(r.spec.ConfigFile)
	require.NoError(t, err)
	require.NoError(t, r.Coordinator.Store.CleanupSnapshotWork(context.Background(), id))
	require.NoDirExists(t, r.work(job.ID))
	require.DirExists(t, r.work(unrelated.ID))
	_, err = r.Coordinator.Store.Get(job.ID)
	require.NoError(t, err, "恢复历史不随暂存文件删除")
	after, err := os.ReadFile(r.spec.ConfigFile)
	require.NoError(t, err)
	require.Equal(t, liveConfig, after)
	unrelated.Status, unrelated.RollbackSnapshotID = "attention_required", id
	require.NoError(t, r.Coordinator.Store.Save(unrelated))
	require.Error(t, r.Coordinator.Store.CleanupSnapshotWork(context.Background(), id))
	require.DirExists(t, r.work(unrelated.ID))
}

func TestCurrentSnapshotDeletionRejectsSymlinkedWork(t *testing.T) {
	r, _, _ := currentFixture(t)
	id := uuid.NewString()
	job := snapshots.ManagedJob{ID: uuid.NewString(), SnapshotID: id, Status: "failed", Kind: "restore"}
	require.NoError(t, r.Coordinator.Store.Save(job))
	require.NoError(t, os.MkdirAll(filepath.Dir(r.work(job.ID)), 0700))
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "keep"), []byte("live data"), 0600))
	require.NoError(t, os.Symlink(outside, r.work(job.ID)))
	require.Error(t, r.Coordinator.Store.CleanupSnapshotWork(context.Background(), id))
	require.FileExists(t, filepath.Join(outside, "keep"))
}
