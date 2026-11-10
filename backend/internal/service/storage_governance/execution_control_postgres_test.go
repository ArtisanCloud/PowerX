package storagegov

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	snapshot "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	ops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func executionChange(action string, revision int64) ExecutionChange {
	return ExecutionChange{RequestID: uuid.NewString(), ExpectedRevision: revision, Action: action, Reason: "独立验收执行控制", Confirm: true}
}

func TestExecutionNativePostgresResourceEffectGuards(t *testing.T) {
	t.Run("file-deletion", func(t *testing.T) {
		s, c, q, _, dir := lifecycleNative(t)
		ctx := controlRoot()
		p := adoptNativeLifecycle(t, s, c, q)
		p.Enabled = true
		_, e := s.SaveLifecycleConfiguration(ctx, "logs", 1, uuid.NewString(), p)
		require.NoError(t, e)
		applyMonitoring(t, c, q)
		_, e = s.ChangeExecution(ctx, executionChange("pause", 0))
		require.NoError(t, e)
		_, _ = s.opts.Lifecycle.RunWithRevision(ctx, "logs", "owned-global-paused", 2)
		require.FileExists(t, filepath.Join(dir, "app.log.2.gz"))
		require.FileExists(t, filepath.Join(dir, "app.log.3.gz"))
	})
	t.Run("backup-admission", func(t *testing.T) {
		s, c, q, db, p, _, guard := backupNative(t)
		ctx := controlRoot()
		legacy := ops.BackupPolicy{}
		backupApply := backupSettings()
		// 通过现有业务服务创建独立计划，再使用同一正式接管与状态入口。
		legacy.Name = backupApply.Name
		// 其余定义由既有设置映射器写入，避免绕过验证制造无效计划。
		backup.ApplySettings(&legacy, backupApply)
		require.NoError(t, db.Create(&legacy).Error)
		adoptBackup(t, p)
		applyMonitoring(t, c, q)
		yes := true
		backupChange(t, p, "status", legacy.UUID.String(), nil, &yes)
		applyMonitoring(t, c, q)
		called := false
		fn := func(context.Context, *gorm.DB, *ops.BackupPolicy) error { called = true; return nil }
		require.NoError(t, guard.Guard(ctx, legacy.ID, 0, "start", fn))
		require.True(t, called)
		called = false
		_, e := s.ChangeExecution(ctx, executionChange("pause", 0))
		require.NoError(t, e)
		require.Error(t, guard.Guard(ctx, legacy.ID, 0, "start", fn))
		require.False(t, called)
	})
	t.Run("snapshot-admission", func(t *testing.T) {
		s, c, q, _, snap, _, _ := snapshotNative(t)
		ctx := controlRoot()
		p, e := snap.PutPolicy(ctx, "", "owned", snapshot.Policy{IntervalMinutes: 60, RetentionDays: 7, RetentionCount: 2})
		require.NoError(t, e)
		bindAndAdoptSnapshot(t, s, snap)
		applyMonitoring(t, c, q)
		snapshotChange(t, snap, "current", p.ID, nil, nil)
		applyMonitoring(t, c, q)
		guard := &snapshotPolicyControl{s, snap}
		called := false
		fn := func(snapshot.PolicyEntry) error { called = true; return nil }
		require.NoError(t, guard.Guard(ctx, p.ID, 0, "manual", fn))
		require.True(t, called)
		called = false
		_, e = s.ChangeExecution(ctx, executionChange("pause", 0))
		require.NoError(t, e)
		require.Equal(t, "STORAGE_EXECUTION_PAUSED", dto.CodeOf(guard.Guard(ctx, p.ID, 0, "manual", fn)))
		require.False(t, called)
	})
	t.Run("archive-retention", func(t *testing.T) {
		c, ctrl, _, _ := policyNative(t)
		ctx := controlRoot()
		adoptNativePolicy(t, c, ctrl)
		called := false
		fn := func([]archive.Policy, *archive.Target) error { called = true; return nil }
		require.NoError(t, c.Guard(ctx, archive.Policy{}, "local-retention", fn))
		require.True(t, called)
		called = false
		_, e := c.service.ChangeExecution(ctx, executionChange("pause", 0))
		require.NoError(t, e)
		require.Equal(t, "STORAGE_EXECUTION_PAUSED", dto.CodeOf(c.Guard(ctx, archive.Policy{}, "local-retention", fn)))
		require.False(t, called)
	})
	t.Run("invocation-retention", func(t *testing.T) {
		s, apply, db := retentionNative(t)
		ctx := controlRoot()
		seedRetentionNative(t, db)
		adoptRetentionNative(t, s, apply)
		p := enableRetentionNative(t, s, apply, 1)
		preview, e := s.PreviewDatabaseRetention(ctx, p.Revision)
		require.NoError(t, e)
		before := retentionRows(t, db)
		_, e = s.ChangeExecution(ctx, executionChange("pause", 0))
		require.NoError(t, e)
		_, e = s.RunConfirmedDatabaseRetention(ctx, p.Revision, uuid.NewString(), preview.ID)
		require.Equal(t, "STORAGE_EXECUTION_PAUSED", dto.CodeOf(e))
		require.Equal(t, before, retentionRows(t, db))
	})
}
func TestExecutionNativePostgresPauseWithoutPublisherAndExactReceipt(t *testing.T) {
	s, c, q, _ := monitoringNative(t)
	ctx := controlRoot()
	config, e := s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	require.Equal(t, "unmanaged", config.Mode)
	// 没有开启保护时保持原合同；不会偷偷启用新策略。
	called := 0
	step := func() error {
		return c.WithExecution(ctx, ControlTableMonitoring, func(*m.ControlConfiguration) error { called++; return nil })
	}
	require.NoError(t, step())
	require.Equal(t, 1, called)
	s.opts.ControlWorkerEnabled = false
	in := executionChange("pause", 0)
	out, e := s.ChangeExecution(ctx, in)
	require.NoError(t, e)
	require.Equal(t, "paused", out.Configuration.Mode)
	require.False(t, out.Configuration.Confirmed)
	require.False(t, out.Configuration.WorkerReady)
	require.Equal(t, "STORAGE_EXECUTION_PAUSED", dto.CodeOf(step()))
	require.Equal(t, 1, called)
	_, e = s.ChangeExecution(ctx, executionChange("resume", 1))
	require.Error(t, e)
	applyMonitoring(t, c, q)
	s.opts.ControlWorkerEnabled = true
	config, e = s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	require.True(t, config.Confirmed)
	require.True(t, config.CanResume)
	resume := executionChange("resume", 1)
	out, e = s.ChangeExecution(ctx, resume)
	require.NoError(t, e)
	require.Equal(t, "open", out.Configuration.Mode)
	require.False(t, out.Configuration.Confirmed)
	require.Error(t, step())
	applyMonitoring(t, c, q)
	require.NoError(t, step())
	require.Equal(t, 2, called)
	old, e := s.ExecutionRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, int64(1), old.OperationRevision)
	require.Equal(t, "pause", old.Operation.Action)
	require.Equal(t, "open", old.Configuration.Mode)
	replay, e := s.ChangeExecution(ctx, in)
	require.NoError(t, e)
	require.Equal(t, int64(1), replay.OperationRevision)
	in.Reason = "同 UUID 不同请求"
	_, e = s.ChangeExecution(ctx, in)
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	_, e = s.ChangeExecution(context.Background(), executionChange("pause", 2))
	require.ErrorIs(t, e, ErrRootRequired)
}

func TestExecutionNativePostgresPauseWaitsForBoundedStepAndFencesNext(t *testing.T) {
	s, c, _, _ := monitoringNative(t)
	ctx := controlRoot()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- c.WithExecution(ctx, ControlTableMonitoring, func(*m.ControlConfiguration) error { close(entered); <-release; return nil })
	}()
	<-entered
	paused := make(chan error, 1)
	go func() { _, e := s.ChangeExecution(ctx, executionChange("pause", 0)); paused <- e }()
	select {
	case e := <-paused:
		t.Fatalf("pause returned while previous step held the fence: %v", e)
	case <-time.After(80 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, <-paused)
	var calls atomic.Int64
	for i := 0; i < 5; i++ {
		e := c.WithExecution(ctx, ControlTableMonitoring, func(*m.ControlConfiguration) error { calls.Add(1); return nil })
		require.Error(t, e)
	}
	require.Zero(t, calls.Load())
}

func TestExecutionNativePostgresWatermarkRollbackAndTamperFailClosed(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	in := executionChange("pause", 0)
	_, e := s.ChangeExecution(ctx, in)
	require.NoError(t, e)
	var row m.ControlConfiguration
	require.NoError(t, db.Where("scope_id=? AND domain=?", c.opts.Scope.ID(), string(ControlExecution)).First(&row).Error)
	// 未发布的暂停同样已有主机水位；DB-only 回滚不能重新落回 unmanaged。
	require.NoError(t, db.Unscoped().Where("scope_id=? AND domain=?", c.opts.Scope.ID(), string(ControlExecution)).Delete(&m.ControlConfiguration{}).Error)
	e = c.WithExecution(ctx, ControlTableMonitoring, func(*m.ControlConfiguration) error { t.Fatal("missing gate executed"); return nil })
	require.Error(t, e)
	require.NoError(t, db.Create(&row).Error)
	applyMonitoring(t, c, q)
	_, e = s.ChangeExecution(ctx, executionChange("resume", 1))
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	_, e = s.opts.Budget.Document(ctx, "execution-control-snapshot", func(raw []byte) ([]byte, error) {
		var p ControlPublication
		require.NoError(t, json.Unmarshal(raw, &p))
		p.Document = json.RawMessage(`{"version":9}`)
		return json.Marshal(p)
	})
	require.NoError(t, e)
	e = c.WithExecution(ctx, ControlTableMonitoring, func(*m.ControlConfiguration) error { t.Fatal("tampered gate executed"); return nil })
	require.Error(t, e)
	// 暂停不依赖已损坏的旧发布投影，也不要求消费者在线。
	s.opts.ControlWorkerEnabled = false
	_, e = s.ChangeExecution(ctx, executionChange("pause", 2))
	require.NoError(t, e)
}

func TestExecutionNativePostgresHandoffRequiresConfirmedPausedPoliciesAndSeparateResume(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	_, e := s.ChangeExecution(ctx, executionChange("begin_handoff", 0))
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	_, e = s.ChangeExecution(ctx, executionChange("pause", 1))
	require.Equal(t, "STORAGE_EXECUTION_HANDOFF_LOCKED", dto.CodeOf(e))
	complete := executionChange("complete_handoff", 1)
	_, e = s.ChangeExecution(ctx, complete)
	require.Error(t, e)
	complete.ExternalStopped = true
	complete.AdmittedTasksChecked = true
	// 未知已登记执行域必须阻止交接完成，不能被当成安全停用。
	require.NoError(t, db.Create(&m.ControlConfiguration{TenantID: 0, ScopeID: c.opts.Scope.ID(), Domain: "unknown-legacy", HostID: c.opts.Scope.HostID, FilesystemID: c.opts.Scope.FilesystemID, InstanceID: c.opts.Scope.InstanceID, Revision: 1, Document: []byte(`{}`), Digest: "unknown", ActorID: 7}).Error)
	_, e = s.ChangeExecution(ctx, complete)
	require.Equal(t, "STORAGE_EXECUTION_POLICY_NOT_PAUSED", dto.CodeOf(e))
	require.NoError(t, db.Unscoped().Where("scope_id=? AND domain=?", c.opts.Scope.ID(), "unknown-legacy").Delete(&m.ControlConfiguration{}).Error)
	out, e := s.ChangeExecution(ctx, complete)
	require.NoError(t, e)
	require.Equal(t, "paused", out.Configuration.Mode)
	applyMonitoring(t, c, q)
	e = c.WithExecution(ctx, ControlTableMonitoring, func(*m.ControlConfiguration) error { t.Fatal("handoff completion reopened gate"); return nil })
	require.Error(t, e)
	_, e = s.ChangeExecution(ctx, executionChange("resume", 2))
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	require.NoError(t, c.WithExecution(ctx, ControlTableMonitoring, func(*m.ControlConfiguration) error { return nil }))
}

func TestExecutionNativePostgresSingleConnectionStatusAndConsumerExit(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(controlRoot(), 4*time.Second)
	defer cancel()
	_, e = s.ChangeExecution(ctx, executionChange("pause", 0))
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	_, e = s.ChangeExecution(ctx, executionChange("resume", 1))
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	close(c.done)
	e = c.WithExecution(ctx, ControlTableMonitoring, func(*m.ControlConfiguration) error { t.Fatal("exited consumer executed"); return nil })
	require.Error(t, e)
	out, e := s.ChangeExecution(ctx, executionChange("pause", 2))
	require.NoError(t, e)
	require.False(t, out.Configuration.WorkerReady)
}

func TestExecutionNativePostgresRealTableCleanupAndHandoffInventory(t *testing.T) {
	s, apply, db := systemRetentionNative(t)
	ctx := controlRoot()
	seedSystemNative(t, db)
	adoptSystemNative(t, s, apply, repo.ProbeRetentionID)
	policy := enableSystemNative(t, s, apply, repo.ProbeRetentionID, 1)
	preview, e := s.PreviewSystemTableRetention(ctx, repo.ProbeRetentionID, policy.Revision)
	require.NoError(t, e)
	_, e = s.ChangeExecution(ctx, executionChange("begin_handoff", 0))
	require.NoError(t, e)
	apply()
	before := systemRows(t, db)
	_, e = s.RunSystemTableRetention(ctx, repo.ProbeRetentionID, policy.Revision, uuid.NewString(), preview.ID)
	require.Equal(t, "STORAGE_EXECUTION_PAUSED", dto.CodeOf(e))
	require.Equal(t, before, systemRows(t, db), "global pause must preserve real expired rows")
	complete := executionChange("complete_handoff", 1)
	complete.ExternalStopped, complete.AdmittedTasksChecked = true, true
	_, e = s.ChangeExecution(ctx, complete)
	require.Equal(t, "STORAGE_EXECUTION_POLICY_NOT_PAUSED", dto.CodeOf(e))
	no := false
	_, e = s.ChangeSystemTableRetention(ctx, repo.ProbeRetentionID, SystemTableRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: 2, Action: "status", Enabled: &no, Confirm: true})
	require.NoError(t, e, "policy pause must remain editable under global handoff")
	_, e = s.ChangeExecution(ctx, complete)
	require.Equal(t, "STORAGE_EXECUTION_POLICY_NOT_PAUSED", dto.CodeOf(e), "unpublished resource pause cannot complete handoff")
	apply()
	_, e = s.ChangeExecution(ctx, complete)
	require.NoError(t, e)
	apply()
	// 完成交接后、恢复前再次启用一项策略，恢复必须被拒绝。
	enableSystemNative(t, s, apply, repo.ProbeRetentionID, 3)
	_, e = s.ChangeExecution(ctx, executionChange("resume", 2))
	require.Equal(t, "STORAGE_EXECUTION_POLICY_NOT_PAUSED", dto.CodeOf(e))
	_, e = s.ChangeSystemTableRetention(ctx, repo.ProbeRetentionID, SystemTableRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: 4, Action: "status", Enabled: &no, Confirm: true})
	require.NoError(t, e)
	apply()
	_, e = s.ChangeExecution(ctx, executionChange("resume", 2))
	require.NoError(t, e)
	apply()
	policy = enableSystemNative(t, s, apply, repo.ProbeRetentionID, 5)
	preview, e = s.PreviewSystemTableRetention(ctx, repo.ProbeRetentionID, policy.Revision)
	require.NoError(t, e)
	_, e = s.RunSystemTableRetention(ctx, repo.ProbeRetentionID, policy.Revision, uuid.NewString(), preview.ID)
	require.NoError(t, e)
	require.Less(t, systemRows(t, db)[0], before[0], "separate resume and enable allow the approved deletion")
}

func TestCoreReleaseHandoffBlockedKeepsEmergencyPause(t *testing.T) {
	s, _, _, _ := monitoringNative(t)
	ctx := controlRoot()
	s.opts.LegacyHandoffBlocked = true
	status, e := s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	require.False(t, status.LegacyHandoffAvailable)
	for _, action := range []string{"begin_handoff", "complete_handoff"} {
		_, e = s.ChangeExecution(ctx, executionChange(action, 0))
		require.Equal(t, "STORAGE_HANDOFF_RELEASE_NOT_ACCEPTED", dto.CodeOf(e))
	}
	pause := executionChange("pause", 0)
	result, e := s.ChangeExecution(ctx, pause)
	require.NoError(t, e)
	require.Equal(t, "paused", result.Configuration.Mode)
	require.False(t, result.Configuration.LegacyHandoffAvailable)
	repeated, e := s.ChangeExecution(ctx, pause)
	require.NoError(t, e)
	require.Equal(t, result.RequestID, repeated.RequestID)
}
