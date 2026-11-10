package storagegov

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRetentionHistoryNativeForeignInvalidAndPauseBoundaries(t *testing.T) {
	s, jobs, db, store, _, apply, create := retainRecoverySetup(t)
	ctx := controlRoot()
	var e error
	for _, claims := range []*reqctx.CoreXClaims{
		{UserID: 9, Scope: "access"},
		{UserID: 9, IsRoot: true, Scope: "access", Platforms: []string{"api_key"}},
		{UserID: 9, IsRoot: true, Scope: "access", Platforms: []string{"service_actor"}},
		{UserID: 9, IsRoot: true, Scope: "access", PluginID: "com.powerx.plugins.owned-fixture"},
	} {
		caller := reqctx.WithClaims(ctx, claims)
		_, e = jobs.RetentionHistory(caller, 1, 20, "all")
		require.Equal(t, "BACKUP_ROOT_REQUIRED", dto.CodeOf(e))
		_, e = jobs.ImportRetentionHistory(caller, historyInput(strings.Repeat("a", 64)), func(string, backup.RetentionImportReceipt) error { return nil })
		require.Equal(t, "BACKUP_ROOT_REQUIRED", dto.CodeOf(e))
	}
	for _, args := range []struct {
		page, size int
		filter     string
	}{{0, 20, "all"}, {1, 51, "all"}, {1, 20, "foreign"}} {
		_, e = jobs.RetentionHistory(ctx, args.page, args.size, args.filter)
		require.Error(t, e)
	}
	f := create()
	row := seedLegacyHistory(t, db, strings.Repeat("f", 64), f, "retained")
	originalDigest := row.InputDigest
	v, e := jobs.RetentionHistory(ctx, 1, 20, "all")
	require.NoError(t, e)
	require.Equal(t, 1, v.Foreign)
	require.True(t, v.Complete, "only current scope is covered")
	require.False(t, v.RestoreReady)
	inv, e := store.RetentionInventory(ctx)
	require.NoError(t, e)
	_, e = backup.CheckRetentionRecoveryCoverage(ctx, db, coremodel.PowerXSchema, inv.Records)
	require.Equal(t, "BACKUP_HISTORY_COVERAGE_REQUIRED", dto.CodeOf(e))
	require.NoError(t, db.Model(&row).Update("scope_fingerprint", s.opts.Control.opts.Scope.ID()).Error)
	require.NoError(t, db.Model(&row).Update("input_digest", strings.Repeat("0", 64)).Error)
	v, e = jobs.RetentionHistory(ctx, 1, 20, "invalid")
	require.NoError(t, e)
	require.Equal(t, 1, v.Invalid)
	require.False(t, v.CanImport)
	out, e := jobs.ImportRetentionHistory(ctx, historyInput(v.Snapshot), func(string, backup.RetentionImportReceipt) error { return nil })
	require.NoError(t, e)
	require.Equal(t, "rejected", out.State)
	require.Equal(t, "BACKUP_HISTORY_RECORD_INVALID", out.Problem)
	require.NoError(t, db.Model(&row).Update("input_digest", originalDigest).Error)
	state, e := s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	_, e = s.ChangeExecution(ctx, executionChange("resume", state.DesiredRevision))
	require.NoError(t, e)
	apply()
	v, e = jobs.RetentionHistory(ctx, 1, 20, "all")
	require.NoError(t, e)
	require.False(t, v.PauseReady)
	require.False(t, v.CanImport)
	_, e = jobs.ImportRetentionHistory(ctx, historyInput(v.Snapshot), func(string, backup.RetentionImportReceipt) error { return nil })
	require.Error(t, e, "running executor cannot import")
	assertRetainSources(t, db, f, false)
}

func TestRetentionHistoryNativeSQLRestoreKeepsImportedHistoryAndProof(t *testing.T) {
	if os.Getenv("POWERX_STORAGE_RETENTION_RESTORE_TEST") != "1" {
		t.Skip("owned native PostgreSQL dump/restore only")
	}
	s, jobs, db, store, _, _, create := retainRecoverySetup(t)
	ctx := controlRoot()
	f := create()
	seedLegacyHistory(t, db, s.opts.Control.opts.Scope.ID(), f, "checking")
	dsn, bin := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN"), os.Getenv("POWERX_STORAGE_TEST_PG_BIN")
	require.Contains(t, dsn, "/tmp/powerx-control-owned-")
	require.True(t, filepath.IsAbs(bin))
	file := filepath.Join(t.TempDir(), "legacy-receipts.dump")
	run := func(name string, args ...string) {
		t.Helper()
		bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		output, e := exec.CommandContext(bounded, filepath.Join(bin, name), args...).CombinedOutput()
		require.NoError(t, e, "%s: %s", name, output)
	}
	run("pg_dump", "--dbname="+dsn, "--format=custom", "--schema="+coremodel.PowerXSchema, "--file="+file)
	later := create()
	seedLegacyHistory(t, db, s.opts.Control.opts.Scope.ID(), later, "retained")
	v, e := jobs.RetentionHistory(ctx, 1, 20, "all")
	require.NoError(t, e)
	in := historyInput(v.Snapshot)
	r, e := jobs.ImportRetentionHistory(ctx, in, func(string, backup.RetentionImportReceipt) error { return nil })
	require.NoError(t, e)
	require.True(t, r.Complete)
	_, e = store.BeginChecked(ctx, uuid.NewString(), uuid.NewString(), func(records map[string]recovery.RetentionRecord) error {
		count, e := backup.CheckRetentionRecoveryCoverage(ctx, db, coremodel.PowerXSchema, records)
		require.Equal(t, 2, count)
		return e
	})
	require.NoError(t, e)
	run("pg_restore", "--clean", "--if-exists", "--exit-on-error", "--no-owner", "--dbname="+dsn, file)
	proof, e := jobs.RetentionImportRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, r, proof)
	for _, item := range []retainRecoveryArtifact{f, later} {
		history, e := jobs.RetentionRequest(ctx, item.in.RequestID)
		require.NoError(t, e)
		require.Equal(t, "maintenance", history.RecordSource)
		require.False(t, history.CanResume)
	}
	assertRetainSources(t, db, f, false)
	var count int64
	require.NoError(t, db.Model(&m.BackupCleanupRetention{}).Count(&count).Error)
	require.Equal(t, int64(1), count, "actual SQL restored to older snapshot")
}

func seedLegacyHistory(t *testing.T, db *gorm.DB, scope string, f retainRecoveryArtifact, state string) m.BackupCleanupRetention {
	t.Helper()
	raw, e := json.Marshal(struct {
		Artifact uint64
		Input    backup.CleanupRetainInput
	}{f.a.ID, f.in})
	require.NoError(t, e)
	sum := sha256.Sum256(raw)
	r := m.BackupCleanupRetention{RequestID: uuid.MustParse(f.in.RequestID), ArtifactID: f.a.ID, JobID: f.j.ID, PolicyID: f.j.PolicyID, CleanupToken: f.in.ExpectedToken, ActorID: 9, InputDigest: hex.EncodeToString(sum[:]), ReviewVersion: f.in.ReviewVersion, ScopeFingerprint: scope, Reason: f.in.Reason, State: state}
	r.CreatedAt = time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	if state != "checking" {
		now := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
		r.CompletedAt = &now
		if state == "rejected" {
			r.Problem = "BACKUP_RETAIN_REVIEW_CHANGED"
		}
	}
	require.NoError(t, db.Create(&r).Error)
	return r
}
func historyInput(snapshot string) backup.RetentionImportInput {
	return backup.RetentionImportInput{RequestID: uuid.NewString(), ExpectedSnapshot: snapshot, Reason: "copy owned legacy historical identities", Confirm: true}
}

func TestRetentionHistoryNativeSourceLimitRefusesWholeBatch(t *testing.T) {
	s, jobs, db, store, _, _, create := retainRecoverySetup(t)
	f := create()
	seed := seedLegacyHistory(t, db, s.opts.Control.opts.Scope.ID(), f, "checking")
	rows := make([]m.BackupCleanupRetention, recovery.MaxRetentionRecords)
	for i := range rows {
		row := seed
		row.ID, row.UUID, row.RequestID = 0, uuid.New(), uuid.New()
		row.CleanupToken = uuid.NewString()
		in := f.in
		in.RequestID = row.RequestID.String()
		in.ExpectedToken = row.CleanupToken
		raw, e := json.Marshal(struct {
			Artifact uint64
			Input    backup.CleanupRetainInput
		}{f.a.ID, in})
		require.NoError(t, e)
		sum := sha256.Sum256(raw)
		row.InputDigest = hex.EncodeToString(sum[:])
		rows[i] = row
	}
	require.NoError(t, db.CreateInBatches(&rows, 200).Error)
	v, e := jobs.RetentionHistory(controlRoot(), 1, 20, "all")
	require.NoError(t, e)
	require.Equal(t, recovery.MaxRetentionRecords+1, v.Total)
	require.Equal(t, "BACKUP_HISTORY_SOURCE_LIMIT", v.Problem)
	require.False(t, v.CanImport)
	r, e := jobs.ImportRetentionHistory(controlRoot(), historyInput(v.Snapshot), func(string, backup.RetentionImportReceipt) error { return nil })
	require.NoError(t, e)
	require.Equal(t, "rejected", r.State)
	require.Equal(t, "BACKUP_HISTORY_SOURCE_LIMIT", r.Problem)
	inv, e := store.RetentionInventory(controlRoot())
	require.NoError(t, e)
	require.Empty(t, inv.Records)
	_, e = backup.CheckRetentionRecoveryCoverage(controlRoot(), db, coremodel.PowerXSchema, inv.Records)
	require.Equal(t, "BACKUP_HISTORY_SOURCE_LIMIT", dto.CodeOf(e))
	assertRetainSources(t, db, f, false)
}
func TestRetentionHistoryNativeInventoryImportProofAndRecoveryCoverage(t *testing.T) {
	s, jobs, db, store, _, _, create := retainRecoverySetup(t)
	ctx := controlRoot()
	f1, f2, f3 := create(), create(), create()
	rows := []m.BackupCleanupRetention{seedLegacyHistory(t, db, s.opts.Control.opts.Scope.ID(), f1, "retained"), seedLegacyHistory(t, db, s.opts.Control.opts.Scope.ID(), f2, "checking"), seedLegacyHistory(t, db, s.opts.Control.opts.Scope.ID(), f3, "rejected")}
	v, e := jobs.RetentionHistory(ctx, 1, 20, "all")
	require.NoError(t, e)
	require.Equal(t, 3, v.Total)
	require.Equal(t, 3, v.Missing)
	require.False(t, v.Complete)
	require.True(t, v.CanImport)
	require.Equal(t, 1, v.Unknown)
	inv, e := store.RetentionInventory(ctx)
	require.NoError(t, e)
	_, e = backup.CheckRetentionRecoveryCoverage(ctx, db, coremodel.PowerXSchema, inv.Records)
	require.Equal(t, "BACKUP_HISTORY_COVERAGE_REQUIRED", dto.CodeOf(e))
	in := historyInput(v.Snapshot)
	operator := reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 2, IsRoot: true, Scope: "access"})
	events := []string{}
	r, e := jobs.ImportRetentionHistory(operator, in, func(name string, r backup.RetentionImportReceipt) error {
		events = append(events, name)
		require.Equal(t, in.RequestID, r.RequestID)
		return nil
	})
	require.NoError(t, e)
	require.Equal(t, "completed", r.State)
	require.True(t, r.Complete)
	require.Equal(t, 3, r.Imported)
	require.Equal(t, 1, r.Unknown)
	require.Equal(t, "2", r.ActorID)
	require.Contains(t, events, "accepted")
	replay, e := jobs.ImportRetentionHistory(operator, in, func(string, backup.RetentionImportReceipt) error {
		t.Fatal("completed import performed work")
		return nil
	})
	require.NoError(t, e)
	require.Equal(t, r.RequestID, replay.RequestID)
	changed := in
	changed.Reason = "different explicit input"
	_, e = jobs.ImportRetentionHistory(operator, changed, func(string, backup.RetentionImportReceipt) error { return nil })
	require.Equal(t, "BACKUP_RETAIN_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	_, e = jobs.ImportRetentionHistory(ctx, in, func(string, backup.RetentionImportReceipt) error { return nil })
	require.Equal(t, "BACKUP_RETAIN_ACTOR_MISMATCH", dto.CodeOf(e))
	v, e = jobs.RetentionHistory(ctx, 1, 2, "covered")
	require.NoError(t, e)
	require.Equal(t, 3, v.Covered)
	require.Len(t, v.Items, 2)
	require.True(t, v.Complete)
	require.True(t, v.RestoreReady)
	for i, before := range rows {
		var after m.BackupCleanupRetention
		require.NoError(t, db.Where("request_id=?", before.RequestID).First(&after).Error)
		require.Equal(t, before.ActorID, after.ActorID)
		require.Equal(t, before.State, after.State)
		assertRetainSources(t, db, []retainRecoveryArtifact{f1, f2, f3}[i], false)
		external, e := store.Retention(ctx, before.RequestID.String())
		require.NoError(t, e)
		require.Equal(t, uint64(9), external.ActorID)
	}
	unknown, e := jobs.RetentionRequest(ctx, f2.in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "checking", unknown.State)
	require.False(t, unknown.CanResume)
	inv, e = store.RetentionInventory(ctx)
	require.NoError(t, e)
	count, e := backup.CheckRetentionRecoveryCoverage(ctx, db, coremodel.PowerXSchema, inv.Records)
	require.NoError(t, e)
	require.Equal(t, 3, count)
	// SQL replay/removal cannot erase imported identities or authorize protection.
	_, e = store.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, db.Migrator().DropTable(&m.BackupCleanupRetention{}))
	read, e := jobs.RetentionImportRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.True(t, read.Complete)
	for _, before := range rows {
		old, e := jobs.RetentionRequest(ctx, before.RequestID.String())
		require.NoError(t, e)
		require.Equal(t, before.State, old.State)
		require.Equal(t, "maintenance", old.RecordSource)
	}
}
func TestRetentionHistoryNativeSnapshotDriftConflictAndAtomicFailure(t *testing.T) {
	for _, kind := range []string{"drift", "conflict", "file-failure", "accepted-loss", "epoch"} {
		t.Run(kind, func(t *testing.T) {
			s, jobs, db, store, dir, _, create := retainRecoverySetup(t)
			ctx := controlRoot()
			f := create()
			row := seedLegacyHistory(t, db, s.opts.Control.opts.Scope.ID(), f, "retained")
			if kind == "conflict" {
				conflict := recovery.RetentionRecord{RequestID: row.RequestID.String(), Scope: row.ScopeFingerprint, InputDigest: row.InputDigest, ReviewVersion: row.ReviewVersion, ArtifactID: row.ArtifactID, ActorID: 8, Token: row.CleanupToken, Reason: row.Reason, CreatedAt: row.CreatedAt, State: "checking"}
				_, e := store.ReserveRetention(ctx, conflict)
				require.NoError(t, e)
			}
			v, e := jobs.RetentionHistory(ctx, 1, 20, "all")
			require.NoError(t, e)
			if kind == "conflict" {
				require.Equal(t, 1, v.Conflicts)
				require.False(t, v.CanImport)
			}
			input := historyInput(v.Snapshot)
			if kind == "drift" {
				require.NoError(t, db.Model(&row).Update("reason", "changed after preview").Error)
			}
			path, backupPath := filepath.Join(dir, "storage-retention-ledger.json"), filepath.Join(dir, "owned-ledger-before-fault")
			armed, faulted := false, false
			name := "owned_import_checkpoint_fault"
			if kind == "file-failure" {
				require.NoError(t, db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
					if !armed || faulted {
						return
					}
					if _, ok := tx.Statement.Dest.(*[]m.BackupCleanupRetention); !ok {
						return
					}
					require.NoError(t, os.Rename(path, backupPath))
					require.NoError(t, os.Mkdir(path, 0700))
					faulted = true
				}))
			}
			out, e := jobs.ImportRetentionHistory(ctx, input, func(_ string, _ backup.RetentionImportReceipt) error {
				armed = true
				if kind == "accepted-loss" {
					return errors.New("owned disconnected client")
				}
				if kind == "epoch" {
					_, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
					require.NoError(t, e)
				}
				return nil
			})
			if kind == "file-failure" {
				db.Callback().Query().Remove(name)
				require.True(t, faulted)
				require.Error(t, e)
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Rename(backupPath, path))
				r, e := jobs.RetentionImportRequest(ctx, input.RequestID)
				require.NoError(t, e)
				require.Equal(t, "pending", r.State)
				_, e = store.Retention(ctx, row.RequestID.String())
				require.ErrorIs(t, e, recovery.ErrRetentionNotFound)
				out, e = jobs.ImportRetentionHistory(ctx, input, func(string, backup.RetentionImportReceipt) error { return nil })
				require.NoError(t, e)
				require.True(t, out.Complete)
			} else if kind == "accepted-loss" {
				require.Error(t, e)
				out, e = jobs.RetentionImportRequest(ctx, input.RequestID)
				require.NoError(t, e)
				require.Equal(t, "pending", out.State)
				out, e = jobs.ImportRetentionHistory(ctx, input, func(string, backup.RetentionImportReceipt) error { return nil })
				require.NoError(t, e)
				require.True(t, out.Complete)
			} else {
				require.NoError(t, e)
				require.Equal(t, "rejected", out.State)
				require.False(t, out.Complete)
				_, e = store.Retention(ctx, row.RequestID.String())
				if kind != "conflict" {
					require.ErrorIs(t, e, recovery.ErrRetentionNotFound)
				}
			}
			assertRetainSources(t, db, f, false)
		})
	}
}
