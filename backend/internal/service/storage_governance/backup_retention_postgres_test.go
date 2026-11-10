package storagegov

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupRetentionNativeControlReceiptsAndFaults(t *testing.T) {
	s, c, q, db, p, j, authority := backupNative(t)
	ownedRecoveryAnchor(t, s)
	ctx := controlRoot()
	require.NoError(t, database.MigrateBackupCleanupRetentionModels(db))
	require.NoError(t, db.AutoMigrate(&model.RestoreDrillRecord{}))
	policy := model.BackupPolicy{}
	backup.ApplySettings(&policy, backupSettings())
	require.NoError(t, db.Create(&policy).Error)
	adoptBackup(t, p)
	applyMonitoring(t, c, q)
	_, e := s.ChangeExecution(ctx, executionChange("pause", 0))
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	dir, e := filepath.EvalSymlinks(filepath.Dir(os.Getenv("POWERX_OPS_BACKUP_ARTIFACT_DIR")))
	require.NoError(t, e)
	root := filepath.Join(dir, "artifacts")
	require.NoError(t, os.MkdirAll(root, 0700))
	type fixture struct {
		artifact model.BackupArtifact
		job      model.BackupJob
		path     string
		data     []byte
		input    backup.CleanupRetainInput
	}
	create := func() fixture {
		job := model.BackupJob{PolicyID: policy.ID, Status: model.BackupJobStatusSuccess, TriggerType: model.BackupTriggerTypeManual, Operator: "owned"}
		require.NoError(t, db.Create(&job).Error)
		path := filepath.Join(root, uuid.NewString()+".dump")
		data := []byte(strings.Repeat("fixture", 100))
		sum := sha256.Sum256(data)
		a := model.BackupArtifact{JobID: job.ID, StorageURI: "file://" + path, SizeBytes: int64(len(data)), Checksum: hex.EncodeToString(sum[:]), CleanupState: "unconfirmed", CleanupToken: uuid.NewString()}
		require.NoError(t, os.WriteFile(path, data, 0600))
		require.NoError(t, db.Create(&a).Error)
		manifest, _ := json.Marshal(map[string]any{"contract": "powerx.database-backup/v1", "source_database": "owned", "job_id": job.ID, "policy_id": policy.ID, "file": filepath.Base(path), "created_at": time.Now().UTC(), "size_bytes": len(data), "sha256": a.Checksum, "format": "postgresql-custom"})
		require.NoError(t, os.WriteFile(path+".json", manifest, 0600))
		require.NoError(t, os.WriteFile(path+".sha256", []byte(a.Checksum+"  "+filepath.Base(path)+"\n"), 0600))
		inspected, e := j.InspectCleanupReview(ctx, a.ID, a.CleanupToken)
		require.NoError(t, e)
		return fixture{a, job, path, data, backup.CleanupRetainInput{RequestID: uuid.NewString(), ExpectedToken: a.CleanupToken, ReviewVersion: inspected.Item.ReviewVersion, Reason: "keep owned backup", Confirm: true}}
	}
	noop := func(backup.CleanupRetainEvent) error { return nil }
	pool, _ := db.DB()
	pool.SetMaxOpenConns(2)
	f := create()
	events := []string{}
	r, e := j.RetainCleanupReview(ctx, f.artifact.ID, f.input, func(v backup.CleanupRetainEvent) error { events = append(events, v.Name); return nil })
	require.NoError(t, e)
	require.Equal(t, "retained", r.State, r)
	require.True(t, r.Protected)
	require.True(t, r.CleanupLockPreserved)
	require.Contains(t, events, "accepted")
	require.Contains(t, events, "progress")
	var job model.BackupJob
	var a model.BackupArtifact
	require.NoError(t, db.First(&job, f.job.ID).Error)
	require.True(t, job.Protected)
	require.NoError(t, db.First(&a, f.artifact.ID).Error)
	require.Equal(t, f.artifact.CleanupState, a.CleanupState)
	require.Equal(t, f.artifact.CleanupToken, a.CleanupToken)
	pool.SetMaxOpenConns(16)
	replay, e := j.RetainCleanupReview(ctx, f.artifact.ID, f.input, func(backup.CleanupRetainEvent) error { t.Error("replay rehashed"); return nil })
	require.NoError(t, e)
	require.Equal(t, r.RequestID, replay.RequestID)
	changed := f.input
	changed.Reason = "other reason"
	_, e = j.RetainCleanupReview(ctx, f.artifact.ID, changed, noop)
	require.Equal(t, "BACKUP_RETAIN_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	other := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 999, IsRoot: true, Scope: "access"})
	_, e = j.RetainCleanupReview(other, f.artifact.ID, f.input, noop)
	require.Equal(t, "BACKUP_RETAIN_ACTOR_MISMATCH", dto.CodeOf(e))
	// Failed validation gets a durable terminal receipt, not an ambiguous cleared form.
	broken := create()
	require.NoError(t, os.WriteFile(broken.path, []byte(strings.Repeat("x", len(broken.data))), 0600))
	failed, e := j.RetainCleanupReview(ctx, broken.artifact.ID, broken.input, noop)
	require.NoError(t, e)
	require.Equal(t, "rejected", failed.State)
	require.Contains(t, failed.Problem, "CHECKSUM_MISMATCH")
	job = model.BackupJob{}
	require.NoError(t, db.First(&job, broken.job.ID).Error)
	require.False(t, job.Protected)
	stale := create()
	stale.input.ReviewVersion = strings.Repeat("a", 64)
	failed, e = j.RetainCleanupReview(ctx, stale.artifact.ID, stale.input, noop)
	require.NoError(t, e)
	require.Equal(t, "BACKUP_RETAIN_REVIEW_CHANGED", failed.Problem)
	// Simulate a process dying after durable admission; a new service resumes the exact UUID/input.
	crashed := create()
	func() {
		defer func() { require.NotNil(t, recover()) }()
		j.RetainCleanupReview(ctx, crashed.artifact.ID, crashed.input, func(backup.CleanupRetainEvent) error { panic("owned crash") })
	}()
	pending, e := j.RetentionRequest(ctx, crashed.input.RequestID)
	require.NoError(t, e)
	require.Equal(t, "checking", pending.State)
	second := backup.NewJobService(db)
	require.NoError(t, second.BindAuthority(authority))
	r, e = second.RetainCleanupReview(ctx, crashed.artifact.ID, crashed.input, noop)
	require.NoError(t, e)
	require.Equal(t, "retained", r.State)
	// An interrupted request is explicitly rejected and leaves the original intent in place.
	interrupted := create()
	r, e = j.RetainCleanupReview(ctx, interrupted.artifact.ID, interrupted.input, func(backup.CleanupRetainEvent) error { return context.Canceled })
	require.NoError(t, e)
	require.Equal(t, "rejected", r.State)
	require.Equal(t, "BACKUP_RETAIN_INTERRUPTED", r.Problem)
	// Competing UUIDs never race protection; exact UUID retries cannot start another reader.
	concurrent := create()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := j.RetainCleanupReview(ctx, concurrent.artifact.ID, concurrent.input, func(v backup.CleanupRetainEvent) error {
			if v.Name == "accepted" {
				close(entered)
				<-release
			}
			return nil
		})
		done <- err
	}()
	<-entered
	_, e = j.RetainCleanupReview(ctx, concurrent.artifact.ID, concurrent.input, noop)
	require.Equal(t, "BACKUP_RETAIN_REQUEST_BUSY", dto.CodeOf(e))
	duplicate := concurrent.input
	duplicate.RequestID = uuid.NewString()
	r, e = j.RetainCleanupReview(ctx, concurrent.artifact.ID, duplicate, noop)
	require.NoError(t, e)
	require.Equal(t, "rejected", r.State)
	require.Equal(t, "BACKUP_RETAIN_OTHER_REQUEST_ACTIVE", r.Problem)
	close(release)
	require.NoError(t, <-done)
	// A pause-to-pause version change during hashing invalidates the captured fence.
	fenced := create()
	once := false
	r, e = j.RetainCleanupReview(ctx, fenced.artifact.ID, fenced.input, func(event backup.CleanupRetainEvent) error {
		if event.Name == "progress" && !once {
			once = true
			cfg, err := s.ExecutionConfiguration(ctx)
			require.NoError(t, err)
			_, err = s.ChangeExecution(ctx, executionChange("pause", cfg.DesiredRevision))
			require.NoError(t, err)
			applyMonitoring(t, c, q)
		}
		return nil
	})
	require.NoError(t, e)
	require.Equal(t, "rejected", r.State)
	require.Equal(t, "BACKUP_RETAIN_FENCE_CHANGED", r.Problem)
	// A pending/running row is not treated as drained merely because a connection lock is free.
	busy := create()
	active := model.BackupJob{PolicyID: policy.ID, Status: model.BackupJobStatusRunning, TriggerType: model.BackupTriggerTypeManual, Operator: "owned"}
	require.NoError(t, db.Create(&active).Error)
	r, e = j.RetainCleanupReview(ctx, busy.artifact.ID, busy.input, noop)
	require.NoError(t, e)
	require.Equal(t, "BACKUP_RETAIN_POLICY_BUSY", r.Problem)
	require.NoError(t, db.Delete(&active).Error)
	// The same tx protects and commits the receipt: injected UPDATE failure cannot leave half of the result.
	rollback := create()
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("owned_retention_failure", func(tx *gorm.DB) {
		values, ok := tx.Statement.Dest.(map[string]interface{})
		if tx.Statement.Table == "ops_backup_cleanup_retentions" && ok && values["state"] == "retained" {
			tx.AddError(errors.New("owned SQL failure"))
		}
	}))
	_, _ = j.RetainCleanupReview(ctx, rollback.artifact.ID, rollback.input, noop)
	require.NoError(t, db.Callback().Update().Remove("owned_retention_failure"))
	job = model.BackupJob{}
	require.NoError(t, db.First(&job, rollback.job.ID).Error)
	require.False(t, job.Protected)
	// This optional schema cannot enter the existing configuration Ready chain.
	require.NoError(t, db.Migrator().DropTable(&model.BackupCleanupRetention{}))
	require.True(t, c.Ready(ctx))
	_, e = j.CleanupReviews(ctx, 1, 20)
	require.NoError(t, e)
	history, e := j.RetentionRequests(ctx, f.artifact.ID, f.artifact.CleanupToken)
	require.NoError(t, e)
	require.NotEmpty(t, history)
	require.Equal(t, "maintenance", history[0].RecordSource)
}
