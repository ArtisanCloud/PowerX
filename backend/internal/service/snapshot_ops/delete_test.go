package snapshot_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSnapshotDeleteRemovesPackageRequestAndRecordOnly(t *testing.T) {
	s, _, liveMedia := fixtureService(t)
	point, other := captureReady(t, s), captureReady(t, s)
	master, err := os.ReadFile(filepath.Join(s.cfg.Directory, "master.key"))
	require.NoError(t, err)
	out, err := s.Delete(rootContext(), point.ID, point.ID, false)
	require.NoError(t, err)
	require.True(t, out.RecordDeleted && out.FilesDeleted)
	for _, path := range []string{s.data(point.ID), s.record("snapshots", point.ID), s.record("requests", point.ID)} {
		require.NoFileExists(t, path)
		_, err := os.Lstat(path)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
	_, err = s.Get(rootContext(), point.ID)
	require.True(t, missingSnapshotRecord(err))
	_, _, err = s.ExportFile(rootContext(), point.ID, "independent-test-passphrase")
	require.True(t, missingSnapshotRecord(err))
	view, err := s.Overview(rootContext())
	require.NoError(t, err)
	require.Len(t, view.Snapshots, 1)
	require.Equal(t, other.ID, view.Snapshots[0].ID)
	require.DirExists(t, s.data(other.ID))
	raw, err := os.ReadFile(liveMedia)
	require.NoError(t, err)
	require.Equal(t, "state A", string(raw))
	after, err := os.ReadFile(filepath.Join(s.cfg.Directory, "master.key"))
	require.NoError(t, err)
	require.Equal(t, master, after)
	audit, err := s.ids("audit")
	require.NoError(t, err)
	require.NotEmpty(t, audit, "最小操作审计不属于被删除的快照展示记录")
}

func TestSnapshotDeleteAuthorizationConfirmationAndProtection(t *testing.T) {
	s, _, _ := fixtureService(t)
	point := captureReady(t, s)
	sts := &reqctx.CoreXClaims{IsRoot: true}
	sts.Issuer = "powerx-sts"
	for _, ctx := range []context.Context{
		context.Background(),
		reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, Platforms: []string{"api_key"}}), true),
		reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), sts), true),
	} {
		_, err := s.Delete(ctx, point.ID, point.ID, true)
		var app *dto.AppError
		require.ErrorAs(t, err, &app)
		require.Equal(t, 403, app.HTTPCode)
	}
	for _, id := range []string{point.ID, "../../outside", "invalid"} {
		_, err := s.Delete(rootContext(), id, "wrong", true)
		require.Error(t, err)
	}
	_, err := s.Protect(rootContext(), point.ID, true)
	require.NoError(t, err)
	_, err = s.Delete(rootContext(), point.ID, point.ID, false)
	require.ErrorContains(t, err, "受保护")
	require.DirExists(t, s.data(point.ID))
	_, err = s.Delete(rootContext(), point.ID, point.ID, true)
	require.NoError(t, err)
}

func TestSnapshotDeleteFailedInterruptedAndPrunedRecords(t *testing.T) {
	for _, status := range []string{"failed", "interrupted", "pruned", "deleting", "delete_failed"} {
		t.Run(status, func(t *testing.T) {
			s, _, _ := fixtureService(t)
			id := uuid.NewString()
			require.NoError(t, atomicJSON(s.record("snapshots", id), Snapshot{ID: id, Status: status}))
			_, err := s.Delete(rootContext(), id, id, false)
			require.NoError(t, err)
			require.NoFileExists(t, s.record("snapshots", id))
		})
	}
}

func TestSnapshotDeleteBlocksActiveCaptureRestoreAndConcurrentOperations(t *testing.T) {
	s, _, _ := fixtureService(t)
	point := captureReady(t, s)
	unlock, err := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	require.NoError(t, err)
	_, err = s.Delete(rootContext(), point.ID, point.ID, false)
	require.Error(t, err)
	unlock()
	point.Status = "capturing"
	require.NoError(t, atomicJSON(s.record("snapshots", point.ID), point))
	_, err = s.Delete(rootContext(), point.ID, point.ID, false)
	require.ErrorContains(t, err, "采集")
	point.Status = "ready"
	require.NoError(t, atomicJSON(s.record("snapshots", point.ID), point))
	restore := Restore{ID: uuid.NewString(), SnapshotID: point.ID, Status: "running"}
	require.NoError(t, atomicJSON(s.record("restores", restore.ID), restore))
	_, err = s.Delete(rootContext(), point.ID, point.ID, false)
	require.ErrorContains(t, err, "正在恢复")
	require.DirExists(t, s.data(point.ID))
}

func TestSnapshotDeleteFencesRecoveryEnqueueAndHeldRollback(t *testing.T) {
	s, _, _ := fixtureService(t)
	point := captureReady(t, s)
	store, err := NewCoordinatorStore(t.TempDir(), []RecoveryTarget{{ID: "local", Label: "Local", Environment: "dev", Mode: "isolated"}})
	require.NoError(t, err)
	store.SnapshotStore = s
	s.opts.RecoveryStore = func() *CoordinatorStore { return store }
	job := ManagedJob{ID: uuid.NewString(), SnapshotID: uuid.NewString(), RollbackSnapshotID: point.ID, Kind: "restore", Status: "attention_required"}
	require.NoError(t, store.Save(job))
	_, err = s.Delete(rootContext(), point.ID, point.ID, true)
	require.ErrorContains(t, err, "恢复任务")
	job.Status = "complete"
	require.NoError(t, store.Save(job))
	store.CleanupSnapshotWork = func(context.Context, string) error {
		_, err := store.EnqueueRestoreMode(rootContext(), point.ID, "local", uuid.NewString(), "new_environment")
		require.Error(t, err, "清理期间不能入队恢复")
		return nil
	}
	_, err = s.Delete(rootContext(), point.ID, point.ID, false)
	require.NoError(t, err)
	_, err = store.EnqueueRestoreMode(rootContext(), point.ID, "local", uuid.NewString(), "new_environment")
	require.True(t, missingSnapshotRecord(err), "陈旧页面不能在清理完成后恢复已删除包")
}

func TestSnapshotDeleteFailureKeepsRetryableRecordAndDoesNotBlockNewCapture(t *testing.T) {
	s, _, _ := fixtureService(t)
	point := captureReady(t, s)
	s.opts.Audit = func(_ context.Context, event AuditEvent) error {
		if event.Action == "delete" && event.Outcome == "started" {
			path := s.record("requests", point.ID)
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Mkdir(path, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(path, "owned-blocker"), []byte("fixture"), 0600))
		}
		return nil
	}
	_, err := s.Delete(rootContext(), point.ID, point.ID, false)
	require.ErrorContains(t, err, "清理未完成")
	failed, err := s.Get(rootContext(), point.ID)
	require.NoError(t, err)
	require.Equal(t, "delete_failed", failed.Status)
	_, _, err = s.ExportFile(rootContext(), point.ID, "independent-test-passphrase")
	require.ErrorContains(t, err, "未完成或已清理")
	s.opts.Audit = nil
	captureReady(t, s)
	require.NoError(t, os.RemoveAll(s.record("requests", point.ID)))
	_, err = s.Delete(rootContext(), point.ID, point.ID, false)
	require.NoError(t, err)
}

func TestSnapshotDeleteRejectsStoreSymlinksAndDoesNotFollowInteriorLinks(t *testing.T) {
	s, _, _ := fixtureService(t)
	point := captureReady(t, s)
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "keep")
	require.NoError(t, os.WriteFile(sentinel, []byte("unrelated data"), 0600))
	actual := s.data(point.ID) + "-saved"
	require.NoError(t, os.Rename(s.data(point.ID), actual))
	require.NoError(t, os.Symlink(outside, s.data(point.ID)))
	_, err := s.Delete(rootContext(), point.ID, point.ID, false)
	require.Error(t, err)
	require.NoError(t, os.Remove(s.data(point.ID)))
	require.NoError(t, os.Rename(actual, s.data(point.ID)))
	require.NoError(t, os.Symlink(outside, filepath.Join(s.data(point.ID), "external-link")))
	_, err = s.Delete(rootContext(), point.ID, point.ID, false)
	require.NoError(t, err)
	raw, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "unrelated data", string(raw))
}

func TestSnapshotDeleteAuditFailureLeavesPackageUntouched(t *testing.T) {
	s, _, _ := fixtureService(t)
	point := captureReady(t, s)
	s.opts.Audit = func(context.Context, AuditEvent) error { return errors.New("audit unavailable") }
	_, err := s.Delete(rootContext(), point.ID, point.ID, false)
	require.ErrorContains(t, err, "audit unavailable")
	got, err := s.Get(rootContext(), point.ID)
	require.NoError(t, err)
	require.Equal(t, "ready", got.Status)
	require.DirExists(t, s.data(point.ID))
}
