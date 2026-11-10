package storagegov

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBackupRetentionLedgerNativeResumedRequestCrossingRecoveryStaysUnknown(t *testing.T) {
	_, jobs, db, store, _, _, create := retainRecoverySetup(t)
	f := create()
	ctx := controlRoot()
	interruptRetain(t, jobs, f, "accepted")
	changed := false
	out, e := jobs.RetainCleanupReview(ctx, f.a.ID, f.in, func(v backup.CleanupRetainEvent) error {
		if v.Name == "progress" && !changed {
			_, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
			require.NoError(t, e)
			changed = true
		}
		return nil
	})
	require.NoError(t, e)
	require.True(t, changed)
	require.Equal(t, "checking", out.State)
	require.True(t, out.RecoveryBlocked)
	require.False(t, out.CanResume)
	var row m.BackupCleanupRetention
	require.NoError(t, db.Where("request_id=?", f.in.RequestID).First(&row).Error)
	require.Equal(t, "checking", row.State)
	independent, e := store.Retention(ctx, f.in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "checking", independent.State)
	assertRetainSources(t, db, f, false)
}

func TestBackupRetentionLedgerNativePreSQLReservationCannotBeReused(t *testing.T) {
	s, jobs, db, _, _, _, create := retainRecoverySetup(t)
	f := create()
	ctx := controlRoot()
	name := "owned_pre_sql_admission_failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "ops_backup_cleanup_retentions" {
			tx.AddError(errors.New("owned SQL admission unavailable"))
		}
	}))
	_, e := jobs.RetainCleanupReview(ctx, f.a.ID, f.in, func(backup.CleanupRetainEvent) error { return nil })
	db.Callback().Create().Remove(name)
	require.Equal(t, "BACKUP_RETAIN_RESULT_UNKNOWN", dto.CodeOf(e))
	var count int64
	require.NoError(t, db.Model(&m.BackupCleanupRetention{}).Count(&count).Error)
	require.Zero(t, count)
	history, e := jobs.RetentionRequest(ctx, f.in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "maintenance", history.RecordSource)
	require.Equal(t, "checking", history.State)
	require.True(t, history.RecoveryBlocked)
	require.False(t, history.CanResume)
	result, e := jobs.RetainCleanupReview(ctx, f.a.ID, f.in, func(backup.CleanupRetainEvent) error {
		t.Fatal("reserved UUID with missing SQL history executed")
		return nil
	})
	require.NoError(t, e)
	require.True(t, result.RecoveryBlocked)
	changed := f.in
	changed.Reason = "cannot replace the original input"
	_, e = jobs.RetainCleanupReview(ctx, f.a.ID, changed, func(backup.CleanupRetainEvent) error { return nil })
	require.Equal(t, "BACKUP_RETAIN_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	assertRetainSources(t, db, f, false)
	// Optional retention failures do not change normal configuration admission.
	require.True(t, s.opts.Control.Ready(ctx))
}

func TestBackupRetentionLedgerNativeCommitBeforeFileCompletionStaysReconcilable(t *testing.T) {
	_, jobs, db, _, dir, _, create := retainRecoverySetup(t)
	f := create()
	ctx := controlRoot()
	path, link := filepath.Join(dir, "storage-retention-ledger.json"), filepath.Join(dir, "owned-extra-link")
	name := "owned_post_sql_pre_ledger_fault"
	linked := false
	require.NoError(t, db.Callback().Update().After("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "ops_backup_cleanup_retentions" || linked {
			return
		}
		fields, ok := tx.Statement.Dest.(map[string]any)
		if !ok || fields["state"] != "retained" {
			return
		}
		require.NoError(t, os.Link(path, link))
		linked = true
	}))
	_, e := jobs.RetainCleanupReview(ctx, f.a.ID, f.in, func(backup.CleanupRetainEvent) error { return nil })
	db.Callback().Update().Remove(name)
	require.True(t, linked)
	require.Equal(t, "BACKUP_RETAIN_RESULT_UNKNOWN", dto.CodeOf(e))
	assertRetainSources(t, db, f, true)
	require.NoError(t, os.Remove(link))
	history, e := jobs.RetentionRequest(ctx, f.in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "retained", history.State)
	require.Equal(t, "database", history.RecordSource)
	// GET did not mutate the independent checking record. Explicit same-UUID
	// POST only completes the historical record, without touching the artifact.
	out, e := jobs.RetainCleanupReview(ctx, f.a.ID, f.in, func(backup.CleanupRetainEvent) error { t.Fatal("terminal SQL replay rehashed"); return nil })
	require.NoError(t, e)
	require.Equal(t, "maintenance", out.RecordSource)
	require.Equal(t, "retained", out.State)
	require.NoError(t, db.Migrator().DropTable(&m.BackupCleanupRetention{}))
	history, e = jobs.RetentionRequest(ctx, f.in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "retained", history.State)
	require.Equal(t, "maintenance", history.RecordSource)
}

func TestBackupRetentionLedgerNativeMissingFileBlocksOnlyRetention(t *testing.T) {
	s, jobs, db, _, dir, _, create := retainRecoverySetup(t)
	f := create()
	ctx := controlRoot()
	require.NoError(t, os.Remove(filepath.Join(dir, "storage-retention-ledger.json")))
	_, e := jobs.RetainCleanupReview(ctx, f.a.ID, f.in, func(backup.CleanupRetainEvent) error { t.Fatal("missing independent proof executed"); return nil })
	require.Equal(t, "BACKUP_RETAIN_LEDGER_UNAVAILABLE", dto.CodeOf(e))
	require.True(t, s.opts.Control.Ready(ctx))
	_, e = s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	_, e = jobs.CleanupReviews(ctx, 1, 20)
	require.NoError(t, e)
	view, e := jobs.InspectCleanupReview(ctx, f.a.ID, f.a.CleanupToken)
	require.NoError(t, e)
	require.Empty(t, view.Item.ReviewVersion)
	assertRetainSources(t, db, f, false)
}

func TestBackupRetentionLedgerNativeUnboundAnchorCannotIssueWritableVersion(t *testing.T) {
	s, jobs, db, _, _, _, create := retainRecoverySetup(t)
	f := create()
	ctx := controlRoot()
	s.opts.RecoveryAnchor = nil
	s.opts.RequireRecoveryAnchor = false
	view, e := jobs.InspectCleanupReview(ctx, f.a.ID, f.a.CleanupToken)
	require.NoError(t, e)
	require.Empty(t, view.Item.ReviewVersion)
	require.True(t, view.ReadOnly)
	_, e = jobs.RetainCleanupReview(ctx, f.a.ID, f.in, func(backup.CleanupRetainEvent) error { t.Fatal("unbound ledger executed"); return nil })
	require.Equal(t, "BACKUP_RETAIN_LEDGER_UNAVAILABLE", dto.CodeOf(e))
	require.True(t, s.opts.Control.Ready(ctx))
	assertRetainSources(t, db, f, false)
}
