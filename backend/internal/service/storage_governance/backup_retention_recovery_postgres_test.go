package storagegov

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type retainRecoveryArtifact struct {
	a  m.BackupArtifact
	j  m.BackupJob
	in backup.CleanupRetainInput
}

func retainRecoverySetup(t *testing.T) (*Service, *backup.JobService, *gorm.DB, *storagerecovery.Store, string, func(), func() retainRecoveryArtifact) {
	t.Helper()
	s, c, q, db, p, jobs, _ := backupNative(t)
	require.NoError(t, database.MigrateBackupCleanupRetentionModels(db))
	require.NoError(t, db.AutoMigrate(&m.RestoreDrillRecord{}))
	store, maintenance := ownedRecoveryAnchor(t, s)
	policy := m.BackupPolicy{}
	backup.ApplySettings(&policy, backupSettings())
	require.NoError(t, db.Create(&policy).Error)
	adoptBackup(t, p)
	apply := func() { applyMonitoring(t, c, q) }
	apply()
	_, e := s.ChangeExecution(controlRoot(), executionChange("pause", 0))
	require.NoError(t, e)
	apply()
	root := os.Getenv("POWERX_OPS_BACKUP_ARTIFACT_DIR")
	require.NoError(t, os.MkdirAll(root, 0700))
	create := func() retainRecoveryArtifact {
		job := m.BackupJob{PolicyID: policy.ID, Status: m.BackupJobStatusSuccess, TriggerType: m.BackupTriggerTypeManual}
		require.NoError(t, db.Create(&job).Error)
		data := []byte("owned recovery binding artifact\n")
		sum := sha256.Sum256(data)
		path := filepath.Join(root, uuid.NewString()+".dump")
		a := m.BackupArtifact{JobID: job.ID, StorageURI: "file://" + path, SizeBytes: int64(len(data)), Checksum: hex.EncodeToString(sum[:]), CleanupState: "unconfirmed", CleanupToken: uuid.NewString()}
		require.NoError(t, os.WriteFile(path, data, 0600))
		require.NoError(t, db.Create(&a).Error)
		raw, _ := json.Marshal(map[string]any{"contract": "powerx.database-backup/v1", "source_database": "owned", "job_id": job.ID, "policy_id": policy.ID, "file": filepath.Base(path), "created_at": time.Now().UTC(), "size_bytes": len(data), "sha256": a.Checksum, "format": "postgresql-custom"})
		require.NoError(t, os.WriteFile(path+".json", raw, 0600))
		require.NoError(t, os.WriteFile(path+".sha256", []byte(a.Checksum+"  "+filepath.Base(path)+"\n"), 0600))
		v, e := jobs.InspectCleanupReview(controlRoot(), a.ID, a.CleanupToken)
		require.NoError(t, e)
		require.Len(t, v.Item.ReviewVersion, 64)
		return retainRecoveryArtifact{a, job, backup.CleanupRetainInput{RequestID: uuid.NewString(), ExpectedToken: a.CleanupToken, ReviewVersion: v.Item.ReviewVersion, Reason: "keep owned recovery artifact", Confirm: true}}
	}
	return s, jobs, db, store, maintenance, apply, create
}

func interruptRetain(t *testing.T, jobs *backup.JobService, f retainRecoveryArtifact, phase string) {
	t.Helper()
	require.Panics(t, func() {
		_, _ = jobs.RetainCleanupReview(controlRoot(), f.a.ID, f.in, func(v backup.CleanupRetainEvent) error {
			if v.Name == phase {
				panic("owned simulated process interruption")
			}
			return nil
		})
	})
}

func assertRetainSources(t *testing.T, db *gorm.DB, f retainRecoveryArtifact, protected bool) {
	t.Helper()
	var a m.BackupArtifact
	var j m.BackupJob
	require.NoError(t, db.First(&a, f.a.ID).Error)
	require.NoError(t, db.First(&j, f.j.ID).Error)
	require.Equal(t, f.a.CleanupToken, a.CleanupToken)
	require.Equal(t, "unconfirmed", a.CleanupState)
	require.Equal(t, protected, j.Protected)
}

func TestBackupRetentionRecoveryNativeGenerationAndHistoricalReplay(t *testing.T) {
	s, jobs, db, store, maintenance, apply, create := retainRecoverySetup(t)
	ctx := controlRoot()
	keep, early, hashing, unsubmitted := create(), create(), create(), create()
	noop := func(backup.CleanupRetainEvent) error { return nil }
	receipt, e := jobs.RetainCleanupReview(ctx, keep.a.ID, keep.in, noop)
	require.NoError(t, e)
	require.Equal(t, "retained", receipt.State)
	interruptRetain(t, jobs, early, "accepted")
	interruptRetain(t, jobs, hashing, "progress")
	var earlyRow m.BackupCleanupRetention
	require.NoError(t, db.Where("request_id=?", early.in.RequestID).First(&earlyRow).Error)
	require.Empty(t, earlyRow.Fence, "must cover interruption before fence capture")
	epoch, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	status, e := s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	require.Equal(t, "recovery", status.Mode)
	assertRetainSources(t, db, early, false)
	assertRetainSources(t, db, hashing, false)
	// Current protection can differ from an old receipt: replay is history only.
	require.NoError(t, db.Model(&m.BackupJob{}).Where("id=?", keep.j.ID).Update("protected", false).Error)
	replay, e := jobs.RetainCleanupReview(ctx, keep.a.ID, keep.in, func(backup.CleanupRetainEvent) error { t.Fatal("historical replay performed work"); return nil })
	require.NoError(t, e)
	require.Equal(t, receipt.RequestID, replay.RequestID)
	require.True(t, replay.Protected, "historical operation fact")
	assertRetainSources(t, db, keep, false)
	finishOwnedRecovery(t, s, apply, epoch.Epoch, status.DesiredRevision)
	for _, f := range []retainRecoveryArtifact{early, hashing, unsubmitted} {
		out, e := jobs.RetainCleanupReview(ctx, f.a.ID, f.in, func(v backup.CleanupRetainEvent) error { require.NotEqual(t, "progress", v.Name); return nil })
		require.NoError(t, e)
		if f.a.ID == unsubmitted.a.ID {
			require.Equal(t, "rejected", out.State)
			require.Equal(t, "BACKUP_RETAIN_REVIEW_CHANGED", out.Problem)
		} else {
			require.Equal(t, "checking", out.State)
			require.True(t, out.RecoveryBlocked)
			require.False(t, out.CanResume)
			require.Equal(t, "maintenance", out.RecordSource)
		}
		assertRetainSources(t, db, f, false)
	}
	fresh, e := jobs.InspectCleanupReview(ctx, early.a.ID, early.a.CleanupToken)
	require.NoError(t, e)
	require.NotEqual(t, early.in.ReviewVersion, fresh.Item.ReviewVersion)
	freshArtifact := create()
	newInput := freshArtifact.in
	out, e := jobs.RetainCleanupReview(ctx, freshArtifact.a.ID, newInput, noop)
	require.NoError(t, e)
	require.Equal(t, "retained", out.State)
	assertRetainSources(t, db, freshArtifact, true)
	// Anchor loss removes writable versions without removing read-only diagnosis/history.
	lost := create()
	require.NoError(t, os.Remove(filepath.Join(maintenance, "storage-recovery-epoch.json")))
	view, e := jobs.InspectCleanupReview(ctx, lost.a.ID, lost.a.CleanupToken)
	require.NoError(t, e)
	require.Empty(t, view.Item.ReviewVersion)
	require.True(t, view.ReadOnly)
	history, e := jobs.RetentionRequest(ctx, keep.in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "retained", history.State)
	out, e = jobs.RetainCleanupReview(ctx, lost.a.ID, lost.in, noop)
	require.Equal(t, "STORAGE_RECOVERY_ANCHOR_UNAVAILABLE", dto.CodeOf(e))
	assertRetainSources(t, db, lost, false)
}

func TestBackupRetentionRecoveryNativeEpochChangesDuringHash(t *testing.T) {
	s, jobs, db, store, _, _, create := retainRecoverySetup(t)
	f := create()
	changed := false
	out, e := jobs.RetainCleanupReview(controlRoot(), f.a.ID, f.in, func(v backup.CleanupRetainEvent) error {
		if v.Name == "progress" && !changed {
			_, err := store.Begin(controlRoot(), uuid.NewString(), uuid.NewString())
			require.NoError(t, err)
			changed = true
		}
		return nil
	})
	require.NoError(t, e)
	require.True(t, changed)
	require.Equal(t, "rejected", out.State)
	require.Equal(t, "BACKUP_RETAIN_PAUSE_REQUIRED", out.Problem)
	assertRetainSources(t, db, f, false)
	state, e := s.ExecutionConfiguration(controlRoot())
	require.NoError(t, e)
	require.Equal(t, "recovery", state.Mode)
}

// Real pg_dump/pg_restore of one owned schema, not a whole installed system restore.
func TestBackupRetentionRecoveryNativeSQLRestoreDoesNotReplayOldRequest(t *testing.T) {
	if os.Getenv("POWERX_STORAGE_RETENTION_RESTORE_TEST") != "1" {
		t.Skip("requires owned PostgreSQL and native pg_dump/pg_restore")
	}
	s, jobs, db, store, _, apply, create := retainRecoverySetup(t)
	ctx := controlRoot()
	f := create()
	interruptRetain(t, jobs, f, "accepted")
	file := filepath.Join(t.TempDir(), "owned-schema.dump")
	bin := os.Getenv("POWERX_STORAGE_TEST_PG_BIN")
	require.True(t, filepath.IsAbs(bin))
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	require.True(t, strings.Contains(dsn, "/tmp/powerx-control-owned-"))
	run := func(name string, args ...string) {
		t.Helper()
		bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(bounded, filepath.Join(bin, name), args...)
		output, e := cmd.CombinedOutput()
		require.NoError(t, e, "%s: %s", name, output)
	}
	run("pg_dump", "--dbname="+dsn, "--format=custom", "--schema="+coremodel.PowerXSchema, "--file="+file)
	noop := func(backup.CleanupRetainEvent) error { return nil }
	out, e := jobs.RetainCleanupReview(ctx, f.a.ID, f.in, noop)
	require.NoError(t, e)
	require.Equal(t, "retained", out.State)
	assertRetainSources(t, db, f, true)
	later := create()
	_, e = jobs.RetainCleanupReview(ctx, later.a.ID, later.in, noop)
	require.NoError(t, e)
	// Use the same independent Begin primitive as managed restore, before SQL replacement.
	epoch, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	run("pg_restore", "--clean", "--if-exists", "--exit-on-error", "--no-owner", "--dbname="+dsn, file)
	assertRetainSources(t, db, f, false)
	restored, e := jobs.RetentionRequest(ctx, f.in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "retained", restored.State)
	require.Equal(t, "maintenance", restored.RecordSource)
	historical, e := jobs.RetentionRequest(ctx, later.in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "retained", historical.State)
	require.Equal(t, "maintenance", historical.RecordSource)
	state, e := s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	require.Equal(t, "recovery", state.Mode)
	finishOwnedRecovery(t, s, apply, epoch.Epoch, state.DesiredRevision)
	out, e = jobs.RetainCleanupReview(ctx, f.a.ID, f.in, func(v backup.CleanupRetainEvent) error { require.NotEqual(t, "progress", v.Name); return nil })
	require.NoError(t, e)
	require.Equal(t, "retained", out.State)
	require.Equal(t, "maintenance", out.RecordSource)
	assertRetainSources(t, db, f, false)
	// Even a receipt absent from the SQL payload cannot reuse its old review version.
	out, e = jobs.RetainCleanupReview(ctx, later.a.ID, later.in, noop)
	require.NoError(t, e)
	require.Equal(t, "retained", out.State)
	changed := later.in
	changed.Reason = "attempt different input after SQL restore"
	_, e = jobs.RetainCleanupReview(ctx, later.a.ID, changed, noop)
	require.Equal(t, "BACKUP_RETAIN_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	var count int64
	require.NoError(t, db.Model(&m.BackupCleanupRetention{}).Where("request_id IN ?", []string{f.in.RequestID, later.in.RequestID}).Count(&count).Error)
	require.EqualValues(t, 1, count, "historical replay must not recreate missing SQL rows")
	t.Log("real schema dump/restore preserved snapshot UUID and independent epoch; no hash/protection replay; post-snapshot identity and result survive independently; changed input conflicts")
}
