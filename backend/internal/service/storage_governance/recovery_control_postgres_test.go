package storagegov

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func ownedRecoveryAnchor(t *testing.T, s *Service) (*storagerecovery.Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "maintenance")
	require.NoError(t, os.Mkdir(dir, 0700))
	store, e := storagerecovery.Initialize(dir, s.opts.Identity.InstanceID)
	require.NoError(t, e)
	require.NoError(t, s.BindRecoveryAnchor(store))
	return store, dir
}
func acknowledgeRecovery(rev int64, epoch string) ExecutionChange {
	in := executionChange("acknowledge_restore", rev)
	in.ExpectedRecoveryEpoch = epoch
	in.ExternalStopped = true
	in.AdmittedTasksChecked = true
	in.EnvironmentChecked = true
	return in
}

func prepareRecovery(rev int64, epoch string) ExecutionChange {
	in := acknowledgeRecovery(rev, epoch)
	in.Action = "prepare_restore"
	return in
}

func TestRecoveryControlNativePostgresJointRollbackStaysBlockedAndRequiresSeparateResume(t *testing.T) {
	s, apply, db := systemRetentionNative(t)
	ctx := controlRoot()
	store, _ := ownedRecoveryAnchor(t, s)
	seedSystemNative(t, db)
	adoptSystemNative(t, s, apply, repo.ProbeRetentionID)
	policy := enableSystemNative(t, s, apply, repo.ProbeRetentionID, 1)
	_, e := s.ChangeExecution(ctx, executionChange("pause", 0))
	require.NoError(t, e)
	apply()
	_, e = s.ChangeExecution(ctx, executionChange("resume", 1))
	require.NoError(t, e)
	apply()
	var old m.ControlConfiguration
	require.NoError(t, db.Where("scope_id=? AND domain=?", s.opts.Control.opts.Scope.ID(), string(ControlExecution)).First(&old).Error)
	oldBinding, e := s.opts.Budget.Document(ctx, "execution-control-binding", nil)
	require.NoError(t, e)
	oldProjection, e := s.opts.Budget.Document(ctx, "execution-control-snapshot", nil)
	require.NoError(t, e)
	_, e = s.ChangeExecution(ctx, executionChange("pause", 2))
	require.NoError(t, e)
	apply()
	// 模拟恢复旧控制数据库正文／版本及配套运维文档；不是整站物理恢复验收。
	require.NoError(t, db.Model(&m.ControlConfiguration{}).Where("tenant_id=0 AND scope_id=? AND domain=?", old.ScopeID, old.Domain).Updates(map[string]any{"revision": old.Revision, "applied_revision": old.AppliedRevision, "document": old.Document, "digest": old.Digest, "actor_id": old.ActorID, "publication_state": old.PublicationState}).Error)
	require.NoError(t, db.Unscoped().Where("tenant_id=0 AND scope_id=? AND domain=? AND revision>?", old.ScopeID, old.Domain, old.Revision).Delete(&m.ControlRevision{}).Error)
	_, e = s.opts.Budget.Document(ctx, "execution-control-binding", func([]byte) ([]byte, error) { return oldBinding, nil })
	require.NoError(t, e)
	_, e = s.opts.Budget.Document(ctx, "execution-control-snapshot", func([]byte) ([]byte, error) { return oldProjection, nil })
	require.NoError(t, e)
	c, e := s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	require.Equal(t, "open", c.Mode)
	require.True(t, c.Confirmed, "joint old copies are internally consistent before a new recovery epoch")
	epoch, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	c, e = s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	require.Equal(t, "recovery", c.Mode)
	require.False(t, c.CanResume)
	require.Equal(t, epoch.Epoch, c.Recovery.Epoch)
	before := systemRows(t, db)
	_, e = s.PreviewSystemTableRetention(ctx, repo.ProbeRetentionID, policy.Revision)
	require.Equal(t, "STORAGE_EXECUTION_PAUSED", dto.CodeOf(e))
	require.Equal(t, before, systemRows(t, db))
	_, e = s.ChangeExecution(ctx, executionChange("resume", 2))
	require.Equal(t, "STORAGE_RECOVERY_REVIEW_REQUIRED", dto.CodeOf(e))
	_, e = s.ChangeExecution(ctx, acknowledgeRecovery(2, epoch.Epoch))
	require.Equal(t, "STORAGE_RECOVERY_REBUILD_REQUIRED", dto.CodeOf(e))
	wrong := prepareRecovery(2, uuid.NewString())
	_, e = s.ChangeExecution(ctx, wrong)
	require.Equal(t, "STORAGE_RECOVERY_EPOCH_CHANGED", dto.CodeOf(e))
	prepared, e := s.ChangeExecution(ctx, prepareRecovery(2, epoch.Epoch))
	require.NoError(t, e)
	require.Greater(t, prepared.OperationRevision, int64(2))
	require.Equal(t, before, systemRows(t, db))
	_, e = s.ChangeExecution(ctx, acknowledgeRecovery(prepared.OperationRevision, epoch.Epoch))
	require.Equal(t, "STORAGE_EXECUTION_PUBLICATION_PENDING", dto.CodeOf(e))
	apply()
	in := acknowledgeRecovery(prepared.OperationRevision, epoch.Epoch)
	out, e := s.ChangeExecution(ctx, in)
	require.NoError(t, e)
	require.Equal(t, "recovery", out.Configuration.Mode, "ack must wait for its own publication")
	apply()
	c, e = s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	require.Equal(t, "paused", c.Mode)
	require.Equal(t, "acknowledged", c.Recovery.State)
	_, e = s.ChangeExecution(ctx, executionChange("resume", out.OperationRevision))
	require.NoError(t, e)
	apply()
	c, e = s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	require.Equal(t, "open", c.Mode)
	status, e := s.SystemTableRetention(ctx, repo.ProbeRetentionID)
	require.NoError(t, e)
	require.False(t, status.Policy.Enabled, "resume must not revive historical enabled flags")
	next, e := store.Begin(ctx, uuid.NewString(), epoch.SnapshotID)
	require.NoError(t, e)
	require.NotEqual(t, epoch.Epoch, next.Epoch)
	receipt, e := s.ExecutionRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, epoch.Epoch, receipt.Operation.ExpectedRecoveryEpoch)
	require.Equal(t, "recovery", receipt.Configuration.Mode, "old acknowledgement cannot clear a newer restore")
}
func TestRecoveryControlNativePostgresUnmanagedAndMissingAnchorFailClosed(t *testing.T) {
	s, c, _, _ := monitoringNative(t)
	ctx := controlRoot()
	store, dir := ownedRecoveryAnchor(t, s)
	epoch, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	out, e := s.ChangeExecution(ctx, prepareRecovery(0, epoch.Epoch))
	require.NoError(t, e)
	require.Equal(t, int64(1), out.OperationRevision)
	require.NoError(t, os.Remove(filepath.Join(dir, "storage-recovery-epoch.json")))
	e = c.WithExecution(ctx, ControlTableMonitoring, func(*m.ControlConfiguration) error { t.Fatal("missing recovery proof executed"); return nil })
	require.Equal(t, "STORAGE_RECOVERY_ANCHOR_UNAVAILABLE", dto.CodeOf(e))
	_, e = s.ExecutionConfiguration(ctx)
	require.Error(t, e)
	receipt, e := s.ExecutionRequest(ctx, out.RequestID)
	require.NoError(t, e)
	require.Equal(t, "unavailable", receipt.Configuration.Mode)
}
func TestRecoveryControlNativePostgresRequiredBootstrapConnectionAndEpochRace(t *testing.T) {
	s, c, _, db := monitoringNative(t)
	ctx := controlRoot()
	s.opts.RequireRecoveryAnchor = true
	e := c.WithExecution(ctx, ControlTableMonitoring, func(*m.ControlConfiguration) error { t.Fatal("unbound mandatory anchor executed"); return nil })
	require.Error(t, e)
	store, _ := ownedRecoveryAnchor(t, s)
	epoch, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	in := acknowledgeRecovery(0, epoch.Epoch)
	in.EnvironmentChecked = false
	_, e = s.ChangeExecution(ctx, in)
	require.Error(t, e)
	_, e = s.ChangeExecution(context.Background(), acknowledgeRecovery(0, epoch.Epoch))
	require.ErrorIs(t, e, ErrRootRequired)
	require.NoError(t, store.WithEpoch(ctx, func(storagerecovery.Epoch) error {
		_, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
		require.ErrorIs(t, e, storagerecovery.ErrBusy)
		return nil
	}))
	_, e = s.ChangeExecution(ctx, prepareRecovery(0, epoch.Epoch))
	require.NoError(t, e)
}
