package storagegov

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func backupNative(t *testing.T) (*Service, *ControlService, *taskqueue.StreamDriver, *gorm.DB, *backup.PolicyService, *backup.JobService, *backupPolicyControl) {
	t.Helper()
	s, c, q, db := monitoringNative(t)
	require.NoError(t, database.MigrateBackupPolicyControlModels(db))
	require.NoError(t, db.AutoMigrate(&model.BackupAlert{}))
	dir, pathErr := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, pathErr)
	scripts := filepath.Join(dir, "scripts")
	require.NoError(t, os.Mkdir(scripts, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(scripts, "backup-db.sh"), []byte("#!/bin/sh\nprintf 'owned fixture backup' > \"$2\"\n"), 0700))
	t.Setenv("POWERX_OPS_SCRIPT_DIR", scripts)
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", filepath.Join(dir, "artifacts"))
	p, j := backup.NewPolicyService(db), backup.NewJobService(db)
	require.NoError(t, s.BindDatabaseBackupControl(p, j, "test"))
	return s, c, q, db, p, j, &backupPolicyControl{s, repo.NewBackupPolicyRepository(db), "test", j.ExecutionFingerprint}
}
func backupSettings() backup.PolicySettings {
	return backup.PolicySettings{Name: "Owned backup", Schedule: "1h", RetentionCount: 1, RetentionDays: 7, RetentionMode: "count", Timezone: "Asia/Shanghai", DrillIntervalDays: 7, TargetRef: "local_dump"}
}
func adoptBackup(t *testing.T, p *backup.PolicyService) backup.PolicyMutation {
	t.Helper()
	conf, e := p.ControlConfiguration(controlRoot())
	require.NoError(t, e)
	out, e := p.AdoptConfiguration(controlRoot(), backup.PolicyAdoption{RequestID: uuid.NewString(), LegacyDigest: conf.LegacyDigest, Confirm: true})
	require.NoError(t, e)
	return out
}
func backupChange(t *testing.T, p *backup.PolicyService, action, id string, settings *backup.PolicySettings, enabled *bool) backup.PolicyMutation {
	t.Helper()
	conf, e := p.ControlConfiguration(controlRoot())
	require.NoError(t, e)
	var rev int64
	for _, r := range conf.Policies {
		if r.UUID == id {
			rev = r.Revision
		}
	}
	out, e := p.ChangeConfiguration(controlRoot(), backup.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: conf.DesiredRevision, ExpectedPolicyRevision: rev, PolicyUUID: id, Action: action, Settings: settings, Enabled: enabled, Confirm: true})
	require.NoError(t, e)
	return out
}
func TestBackupPolicyNativePostgresAdoptionCASCurrentAndGuard(t *testing.T) {
	_, c, q, db, p, _, guard := backupNative(t)
	ctx := controlRoot()
	legacy := model.BackupPolicy{Enabled: true, IsCurrent: true, DrillEnabled: true}
	settings := backupSettings()
	backup.ApplySettings(&legacy, settings)
	legacy.DrillEnabled = true
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, database.MigrateBackupPolicyControlModels(db))
	var before model.BackupPolicy
	require.NoError(t, db.First(&before, legacy.ID).Error)
	require.True(t, before.Enabled)
	require.True(t, before.DrillEnabled)
	adopted := adoptBackup(t, p)
	require.False(t, adopted.Configuration.Policies[0].Enabled)
	require.False(t, adopted.Configuration.Policies[0].Current)
	require.False(t, adopted.Configuration.Policies[0].Settings.DrillEnabled)
	var version cm.ControlRevision
	require.NoError(t, db.First(&version).Error)
	doc, e := decodeBackupPolicies(version.Document, "test")
	require.NoError(t, e)
	require.True(t, doc.Origin[0].Settings.DrillEnabled)
	applyMonitoring(t, c, q)
	yes := true
	enabled := backupChange(t, p, "status", legacy.UUID.String(), nil, &yes)
	called := false
	action := func(context.Context, *gorm.DB, *model.BackupPolicy) error { called = true; return nil }
	require.Error(t, guard.Guard(ctx, legacy.ID, 0, "start", action))
	require.False(t, called)
	applyMonitoring(t, c, q)
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	require.NoError(t, guard.Guard(ctx, legacy.ID, enabled.Configuration.Policies[0].Revision, "start", action))
	require.True(t, called)
	pool.SetMaxOpenConns(16)
	require.Error(t, guard.Guard(ctx, legacy.ID, 0, "scheduled", action))
	backupChange(t, p, "current", legacy.UUID.String(), nil, nil)
	applyMonitoring(t, c, q)
	require.NoError(t, guard.Guard(ctx, legacy.ID, 0, "scheduled", action))
	other := settings
	other.Name = "Second"
	second := backupChange(t, p, "save", "", &other, nil)
	applyMonitoring(t, c, q)
	backupChange(t, p, "status", second.PolicyUUID, nil, &yes)
	applyMonitoring(t, c, q)
	backupChange(t, p, "current", second.PolicyUUID, nil, nil)
	var count int64
	require.NoError(t, db.Model(&model.BackupPolicy{}).Where("is_current = true").Count(&count).Error)
	require.EqualValues(t, 1, count)
	no := false
	backupChange(t, p, "status", second.PolicyUUID, nil, &no)
	called = false
	require.Error(t, guard.Guard(ctx, legacy.ID, 0, "scheduled", action))
	require.False(t, called)
	var current model.BackupPolicy
	require.NoError(t, db.Where("uuid = ?", second.PolicyUUID).First(&current).Error)
	require.False(t, current.IsCurrent)
	e = p.SetPolicyEnabled(ctx, backup.SetPolicyEnabledRequest{PolicyID: legacy.ID, Enabled: true})
	require.Equal(t, "BACKUP_POLICY_CONTROL_REQUIRED", dto.CodeOf(e))
	_, e = backup.NewPolicyService(db).CreatePolicy(ctx, backup.CreatePolicyRequest{Name: "bypass"})
	require.Equal(t, "BACKUP_POLICY_CONTROL_REQUIRED", dto.CodeOf(e))
	_, e = backup.NewJobService(db).TriggerJob(ctx, backup.TriggerJobRequest{PolicyID: legacy.ID})
	require.Equal(t, "BACKUP_POLICY_CONTROL_UNAVAILABLE", dto.CodeOf(e))
}
func TestBackupPolicyNativePostgresConcurrentReplayAndAtomicRollback(t *testing.T) {
	_, c, q, db, p, _, _ := backupNative(t)
	adoptBackup(t, p)
	applyMonitoring(t, c, q)
	ctx := controlRoot()
	settings := backupSettings()
	in := backup.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 1, Action: "save", Settings: &settings, Confirm: true}
	first, e := p.ChangeConfiguration(ctx, in)
	require.NoError(t, e)
	again, e := p.ChangeConfiguration(ctx, in)
	require.NoError(t, e)
	require.Equal(t, first.OperationRevision, again.OperationRevision)
	receipt, e := p.ConfigurationRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, in.RequestID, receipt.PolicyUUID)
	in.ExpectedPolicyRevision = 9
	_, e = p.ChangeConfiguration(ctx, in)
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	applyMonitoring(t, c, q)
	start := make(chan struct{})
	results := make(chan error, 5)
	var wg sync.WaitGroup
	for n := 0; n < 5; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			x := settings
			x.Name = uuid.NewString()
			_, err := p.ChangeConfiguration(ctx, backup.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 2, Action: "save", Settings: &x, Confirm: true})
			results <- err
		}(n)
	}
	close(start)
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		} else {
			require.Equal(t, "STORAGE_CONTROL_REVISION_CONFLICT", dto.CodeOf(e))
		}
	}
	require.Equal(t, 1, wins)
	var count int64
	require.NoError(t, db.Model(&model.BackupPolicy{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	// 使正式 TaskBus 登记失败，验证同事务的业务行及不可变回执全部回滚。
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("owned-register-fail", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.Table, "event_task_histories") {
			tx.AddError(errors.New("owned register failure"))
		}
	}))
	settings.Name = "must rollback"
	_, e = p.ChangeConfiguration(ctx, backup.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 3, Action: "save", Settings: &settings, Confirm: true})
	require.Error(t, e)
	require.NoError(t, db.Callback().Create().Remove("owned-register-fail"))
	require.NoError(t, db.Model(&model.BackupPolicy{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	controlCounts(t, db, 3)
}
func TestBackupPolicyNativePostgresSourceAndRowTamperFence(t *testing.T) {
	s, c, q, db, p, j, guard := backupNative(t)
	adoptBackup(t, p)
	applyMonitoring(t, c, q)
	settings := backupSettings()
	created := backupChange(t, p, "save", "", &settings, nil)
	applyMonitoring(t, c, q)
	yes := true
	backupChange(t, p, "status", created.PolicyUUID, nil, &yes)
	applyMonitoring(t, c, q)
	guard.fingerprint = func() string { return strings.Repeat("0", 64) }
	var row model.BackupPolicy
	require.NoError(t, db.Where("uuid = ?", created.PolicyUUID).First(&row).Error)
	called := false
	action := func(context.Context, *gorm.DB, *model.BackupPolicy) error { called = true; return nil }
	require.Equal(t, "BACKUP_POLICY_SOURCE_CHANGED", dto.CodeOf(guard.Guard(controlRoot(), row.ID, 0, "start", action)))
	require.False(t, called)
	guard.fingerprint = j.ExecutionFingerprint
	require.NoError(t, db.Model(&row).Update("retention_count", 2).Error)
	require.Error(t, guard.Guard(controlRoot(), row.ID, 0, "cleanup", action))
	require.False(t, called)
	require.NoError(t, db.Model(&row).Update("retention_count", 1).Error)
	_, e := s.opts.Budget.Document(controlRoot(), "backup-policy-control", func([]byte) ([]byte, error) { return []byte(`{"version":1}`), nil })
	require.NoError(t, e)
	require.Error(t, guard.Guard(controlRoot(), row.ID, 0, "start", action))
}
func TestBackupPolicyNativePostgresFixtureCaptureRetentionAndIntent(t *testing.T) {
	_, c, q, db, p, j, _ := backupNative(t)
	ctx := controlRoot()
	adoptBackup(t, p)
	applyMonitoring(t, c, q)
	settings := backupSettings()
	settings.RetentionCount = 2
	created := backupChange(t, p, "save", "", &settings, nil)
	applyMonitoring(t, c, q)
	yes := true
	backupChange(t, p, "status", created.PolicyUUID, nil, &yes)
	applyMonitoring(t, c, q)
	var row model.BackupPolicy
	require.NoError(t, db.Where("uuid = ?", created.PolicyUUID).First(&row).Error)
	trigger := func() *model.BackupJob {
		job, e := j.TriggerJob(ctx, backup.TriggerJobRequest{PolicyID: row.ID, ForceSync: true})
		require.NoError(t, e)
		require.Equal(t, model.BackupJobStatusSuccess, job.Status)
		require.Positive(t, job.PolicyRevision)
		return job
	}
	first := trigger()
	require.NoError(t, j.SetProtected(ctx, first.ID, true, "owned", ""))
	second := trigger()
	third := trigger()
	_ = third
	var firstArt, secondArt model.BackupArtifact
	require.NoError(t, db.Where("job_id = ?", first.ID).First(&firstArt).Error)
	require.NoError(t, db.Where("job_id = ?", second.ID).First(&secondArt).Error)
	trigger()
	require.NoError(t, db.Where("job_id = ?", first.ID).First(&model.BackupArtifact{}).Error)
	require.ErrorIs(t, db.Where("id = ?", secondArt.ID).First(&model.BackupArtifact{}).Error, gorm.ErrRecordNotFound)
	require.NoError(t, db.Unscoped().First(&secondArt, secondArt.ID).Error)
	require.Equal(t, "deleted", secondArt.CleanupState)
	require.NotEmpty(t, secondArt.CleanupToken)
	require.NoError(t, j.SetProtected(ctx, first.ID, false, "owned", ""))
	require.NoError(t, db.Model(&firstArt).Updates(map[string]any{"cleanup_state": "pending", "cleanup_token": uuid.NewString()}).Error)
	require.Error(t, j.TriggerCleanup(ctx, "owned", ""))
	require.NoError(t, db.First(&firstArt, firstArt.ID).Error)
	require.Equal(t, "pending", firstArt.CleanupState)
	path := strings.TrimPrefix(firstArt.StorageURI, "file://")
	_, e := os.Stat(path)
	require.NoError(t, e)
	// 暂停提交即挡住清理，无需等旧投影被替换。
	no := false
	backupChange(t, p, "status", created.PolicyUUID, nil, &no)
	require.Error(t, j.TriggerCleanup(ctx, "owned", ""))
	_, e = j.TriggerJob(ctx, backup.TriggerJobRequest{PolicyID: row.ID})
	require.Error(t, e)
}

func TestBackupPolicyNativePostgresPauseDuringCapturePreservesExpiredFile(t *testing.T) {
	_, c, q, db, p, j, _ := backupNative(t)
	ctx := controlRoot()
	adoptBackup(t, p)
	applyMonitoring(t, c, q)
	settings := backupSettings()
	settings.RetentionCount = 3
	out := backupChange(t, p, "save", "", &settings, nil)
	applyMonitoring(t, c, q)
	yes := true
	backupChange(t, p, "status", out.PolicyUUID, nil, &yes)
	applyMonitoring(t, c, q)
	var row model.BackupPolicy
	require.NoError(t, db.Where("uuid = ?", out.PolicyUUID).First(&row).Error)
	first, e := j.TriggerJob(ctx, backup.TriggerJobRequest{PolicyID: row.ID})
	require.NoError(t, e)
	require.Equal(t, model.BackupJobStatusSuccess, first.Status)
	// 用低保留数保存并重新启用，让首份产物在第二次备份完成后成为删除候选。
	settings.RetentionCount = 1
	backupChange(t, p, "save", out.PolicyUUID, &settings, nil)
	applyMonitoring(t, c, q)
	backupChange(t, p, "status", out.PolicyUUID, nil, &yes)
	applyMonitoring(t, c, q)
	dir := os.Getenv("POWERX_OPS_SCRIPT_DIR")
	marker := filepath.Join(dir, "started")
	release := filepath.Join(dir, "continue")
	script := "#!/bin/sh\nprintf ready > '" + marker + "'\nwhile [ ! -f '" + release + "' ]; do sleep 0.02; done\nprintf 'owned paused fixture' > \"$2\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "backup-db.sh"), []byte(script), 0700))
	done := make(chan error, 1)
	go func() {
		job, err := j.TriggerJob(ctx, backup.TriggerJobRequest{PolicyID: row.ID})
		if err == nil && job.Status != model.BackupJobStatusSuccess {
			err = errors.New(job.ErrorMessage)
		}
		done <- err
	}()
	require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, 3*time.Second, 10*time.Millisecond)
	no := false
	backupChange(t, p, "status", out.PolicyUUID, nil, &no)
	require.NoError(t, os.WriteFile(release, []byte("release"), 0600))
	require.NoError(t, <-done)
	var artifact model.BackupArtifact
	require.NoError(t, db.Where("job_id = ?", first.ID).First(&artifact).Error)
	_, e = os.Stat(strings.TrimPrefix(artifact.StorageURI, "file://"))
	require.NoError(t, e)
	var alerts int64
	require.NoError(t, db.Model(&model.BackupAlert{}).Count(&alerts).Error)
	require.Positive(t, alerts)
}
