package storagegov

import (
	"context"
	"errors"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func resolutionSetup(t *testing.T) (*Service, *backup.JobService, *gorm.DB, *recovery.Store, string, func(), retainRecoveryArtifact) {
	t.Helper()
	s, jobs, db, store, dir, apply, create := retainRecoverySetup(t)
	require.NoError(t, database.MigrateBackupCleanupResolutionModels(db))
	f := create()
	interruptRetain(t, jobs, f, "accepted")
	epoch, e := store.Begin(controlRoot(), uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	state, e := s.ExecutionConfiguration(controlRoot())
	require.NoError(t, e)
	finishOwnedRecovery(t, s, apply, epoch.Epoch, state.DesiredRevision)
	return s, jobs, db, store, dir, apply, f
}
func resolutionInput(t *testing.T, jobs *backup.JobService, f retainRecoveryArtifact) backup.CleanupResolutionInput {
	t.Helper()
	v, e := jobs.PreviewCleanupResolution(controlRoot(), f.a.ID, f.a.CleanupToken)
	require.NoError(t, e)
	require.True(t, v.CanResolve, v.Problem)
	return backup.CleanupResolutionInput{RequestID: uuid.NewString(), ExpectedToken: f.a.CleanupToken, ReviewVersion: v.ReviewVersion, HistoryDigest: v.HistoryDigest, Reason: "owned current intent retained, old history unchanged", Confirm: true}
}
func assertResolved(t *testing.T, db *gorm.DB, f retainRecoveryArtifact) {
	t.Helper()
	var a m.BackupArtifact
	var j m.BackupJob
	require.NoError(t, db.First(&a, f.a.ID).Error)
	require.NoError(t, db.First(&j, f.j.ID).Error)
	require.Equal(t, "retained", a.CleanupState)
	require.Equal(t, f.a.CleanupToken, a.CleanupToken)
	require.True(t, j.Protected)
	_, e := os.Stat(a.StorageURI[len("file://"):])
	require.NoError(t, e)
}
func TestCleanupResolutionNativeRetainsCurrentWithoutRewritingUnknownHistory(t *testing.T) {
	_, jobs, db, store, _, _, f := resolutionSetup(t)
	ctx := controlRoot()
	var old m.BackupCleanupRetention
	require.NoError(t, db.Where("request_id=?", f.in.RequestID).First(&old).Error)
	in := resolutionInput(t, jobs, f)
	events := []string{}
	r, e := jobs.ResolveCleanupRetained(ctx, f.a.ID, in, func(v backup.CleanupResolutionEvent) error { events = append(events, v.Name); return nil })
	require.NoError(t, e)
	require.Equal(t, "retained", r.State)
	require.True(t, r.IndependentComplete)
	require.Contains(t, events, "accepted")
	assertResolved(t, db, f)
	var after m.BackupCleanupRetention
	require.NoError(t, db.Where("request_id=?", f.in.RequestID).First(&after).Error)
	require.Equal(t, old, after)
	original, e := jobs.RetentionRequest(ctx, f.in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "checking", original.State)
	require.False(t, original.CanResume)
	_, e = jobs.RetainCleanupReview(ctx, f.a.ID, f.in, func(backup.CleanupRetainEvent) error { t.Fatal("old unknown replayed"); return nil })
	require.NoError(t, e)
	require.NoError(t, db.Where("request_id=?", f.in.RequestID).First(&after).Error)
	require.Equal(t, old, after)
	replay, e := jobs.ResolveCleanupRetained(ctx, f.a.ID, in, func(backup.CleanupResolutionEvent) error { t.Fatal("terminal replay did work"); return nil })
	require.NoError(t, e)
	require.Equal(t, r, replay)
	changed := in
	changed.Reason = "different new body"
	_, e = jobs.ResolveCleanupRetained(ctx, f.a.ID, changed, func(backup.CleanupResolutionEvent) error { return nil })
	require.Equal(t, "BACKUP_RETAIN_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	actor := reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 2, IsRoot: true, Scope: "access"})
	_, e = jobs.ResolveCleanupRetained(actor, f.a.ID, in, func(backup.CleanupResolutionEvent) error { return nil })
	require.Equal(t, "BACKUP_RETAIN_ACTOR_MISMATCH", dto.CodeOf(e))
	inv, e := store.RetentionInventory(ctx)
	require.NoError(t, e)
	count, e := backup.CheckRetentionRecoveryCoverage(ctx, db, coremodel.PowerXSchema, inv.Records)
	require.NoError(t, e)
	require.Equal(t, 2, count)
	list, e := jobs.CleanupReviews(ctx, 1, 20)
	require.NoError(t, e)
	require.Zero(t, list.Total)
	_, e = store.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, db.Model(&m.BackupJob{}).Where("id=?", f.j.ID).Update("protected", false).Error)
	require.NoError(t, db.Model(&m.BackupArtifact{}).Where("id=?", f.a.ID).Update("cleanup_state", "unconfirmed").Error)
	require.NoError(t, db.Migrator().DropTable(&m.BackupCleanupResolution{}))
	history, e := jobs.CleanupResolutionRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "retained", history.State)
	_, e = jobs.ResolveCleanupRetained(ctx, f.a.ID, in, func(backup.CleanupResolutionEvent) error { t.Fatal("restored history replayed"); return nil })
	require.NoError(t, e)
	assertRetainSources(t, db, f, false)
}

func TestCleanupResolutionNativeFaultBoundaries(t *testing.T) {
	for _, kind := range []string{"drift", "active", "policy-lock", "accepted-loss", "epoch", "file-changed", "sql-rollback", "checkpoint-failure", "pre-sql"} {
		t.Run(kind, func(t *testing.T) {
			_, jobs, db, store, dir, _, f := resolutionSetup(t)
			ctx := controlRoot()
			in := resolutionInput(t, jobs, f)
			if kind == "drift" {
				require.NoError(t, db.Model(&f.a).Update("updated_at", time.Now().UTC()).Error)
			}
			if kind == "active" {
				require.NoError(t, db.Create(&m.BackupJob{PolicyID: f.j.PolicyID, Status: m.BackupJobStatusRunning}).Error)
			}
			if kind == "policy-lock" {
				unlock, e := lockOwnedBackupPolicy(db, f.j.PolicyID)
				require.NoError(t, e)
				defer unlock()
			}
			name := "owned_resolution_fault"
			originalLedger := filepath.Join(dir, "storage-retention-ledger.json")
			savedLedger := filepath.Join(dir, "owned-before-fault")
			faulted := false
			if kind == "sql-rollback" || kind == "checkpoint-failure" || kind == "pre-sql" {
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
					if kind == "pre-sql" && tx.Statement.Table == "ops_backup_cleanup_resolutions" {
						tx.AddError(errors.New("owned SQL admission failure"))
					}
				}))
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
					if tx.Statement.Table != "ops_backup_cleanup_resolutions" {
						return
					}
					values, ok := tx.Statement.Dest.(map[string]any)
					if !ok || values["state"] != "retained" {
						return
					}
					if kind == "sql-rollback" {
						tx.AddError(errors.New("owned terminal write failure"))
					}
					if kind == "checkpoint-failure" {
						require.NoError(t, os.Rename(originalLedger, savedLedger))
						require.NoError(t, os.Mkdir(originalLedger, 0700))
						faulted = true
					}
				}))
				defer db.Callback().Create().Remove(name)
				defer db.Callback().Update().Remove(name)
			}
			out, e := jobs.ResolveCleanupRetained(ctx, f.a.ID, in, func(v backup.CleanupResolutionEvent) error {
				if v.Name != "accepted" {
					return nil
				}
				switch kind {
				case "accepted-loss":
					return errors.New("owned disconnected client")
				case "epoch":
					_, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
					require.NoError(t, e)
				case "file-changed":
					require.NoError(t, os.WriteFile(f.a.StorageURI[len("file://"):], []byte("changed"), 0600))
				}
				return nil
			})
			if faulted {
				require.NoError(t, os.Remove(originalLedger))
				require.NoError(t, os.Rename(savedLedger, originalLedger))
			}
			switch kind {
			case "checkpoint-failure":
				require.Error(t, e)
				assertResolved(t, db, f)
				out, e = jobs.ResolveCleanupRetained(ctx, f.a.ID, in, func(backup.CleanupResolutionEvent) error { t.Fatal("checkpoint retried SQL"); return nil })
				require.NoError(t, e)
				require.Equal(t, "retained", out.State)
			case "accepted-loss":
				require.Error(t, e)
				read, err := jobs.CleanupResolutionRequest(ctx, in.RequestID)
				require.NoError(t, err)
				require.True(t, read.CanResume)
				list, listErr := jobs.CleanupResolutionRequests(ctx, f.a.ID, f.a.CleanupToken)
				require.NoError(t, listErr)
				require.Len(t, list, 1)
				require.Equal(t, read, list[0])
				out, e = jobs.ResolveCleanupRetained(ctx, f.a.ID, in, func(backup.CleanupResolutionEvent) error { return nil })
				require.NoError(t, e)
				require.Equal(t, "retained", out.State)
				assertResolved(t, db, f)
			case "epoch", "pre-sql":
				require.Error(t, e)
				read, err := jobs.CleanupResolutionRequest(ctx, in.RequestID)
				require.NoError(t, err)
				require.True(t, read.RecoveryBlocked)
				require.False(t, read.CanResume)
				assertRetainSources(t, db, f, false)
			case "file-changed":
				require.NoError(t, e)
				require.Equal(t, "rejected", out.State)
				assertRetainSources(t, db, f, false)
			case "sql-rollback":
				require.NoError(t, e)
				require.Equal(t, "rejected", out.State)
				assertRetainSources(t, db, f, false)
			default:
				require.Error(t, e)
				assertRetainSources(t, db, f, false)
			}
		})
	}
}

func lockOwnedBackupPolicy(db *gorm.DB, id uint64) (func(), error) {
	pool, e := db.DB()
	if e != nil {
		return nil, e
	}
	conn, e := pool.Conn(context.Background())
	if e != nil {
		return nil, e
	}
	var name string
	if e = conn.QueryRowContext(context.Background(), "SELECT current_database()").Scan(&name); e != nil {
		conn.Close()
		return nil, e
	}
	h := fnv.New32a()
	h.Write([]byte("powerx.backup." + name))
	class := int32(h.Sum32())
	var yes bool
	e = conn.QueryRowContext(context.Background(), "SELECT pg_try_advisory_lock($1,$2)", class, int32(id)).Scan(&yes)
	if e != nil || !yes {
		conn.Close()
		return nil, errors.New("owned fixture lock unavailable")
	}
	return func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1,$2)", class, int32(id))
		conn.Close()
	}, nil
}

func TestCleanupResolutionNativeConcurrentSingleWinner(t *testing.T) {
	_, jobs, db, _, _, _, f := resolutionSetup(t)
	in := resolutionInput(t, jobs, f)
	other := in
	other.RequestID = uuid.NewString()
	var wg sync.WaitGroup
	wg.Add(2)
	results := make(chan backup.CleanupResolutionReceipt, 2)
	for _, input := range []backup.CleanupResolutionInput{in, other} {
		go func(v backup.CleanupResolutionInput) {
			defer wg.Done()
			r, _ := jobs.ResolveCleanupRetained(controlRoot(), f.a.ID, v, func(backup.CleanupResolutionEvent) error { return nil })
			results <- r
		}(input)
	}
	wg.Wait()
	close(results)
	winners := 0
	for r := range results {
		if r.State == "retained" {
			winners++
		}
	}
	require.Equal(t, 1, winners)
	assertResolved(t, db, f)
}

func TestCleanupResolutionNativeSQLReplayKeepsHistoricalOutcomeAndNeedsFreshReview(t *testing.T) {
	if os.Getenv("POWERX_STORAGE_RETENTION_RESTORE_TEST") != "1" {
		t.Skip("owned native pg_dump/pg_restore only")
	}
	s, jobs, db, store, _, apply, f := resolutionSetup(t)
	ctx := controlRoot()
	in := resolutionInput(t, jobs, f)
	require.Panics(t, func() {
		_, _ = jobs.ResolveCleanupRetained(ctx, f.a.ID, in, func(v backup.CleanupResolutionEvent) error {
			if v.Name == "accepted" {
				panic("owned interruption")
			}
			return nil
		})
	})
	bin, dsn := os.Getenv("POWERX_STORAGE_TEST_PG_BIN"), os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	require.True(t, filepath.IsAbs(bin))
	file := filepath.Join(t.TempDir(), "owned-resolution.dump")
	run := func(name string, args ...string) {
		t.Helper()
		c, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		out, e := exec.CommandContext(c, filepath.Join(bin, name), args...).CombinedOutput()
		require.NoError(t, e, "%s: %s", name, out)
	}
	run("pg_dump", "--dbname="+dsn, "--format=custom", "--schema="+coremodel.PowerXSchema, "--file="+file)
	r, e := jobs.ResolveCleanupRetained(ctx, f.a.ID, in, func(backup.CleanupResolutionEvent) error { return nil })
	require.NoError(t, e)
	require.Equal(t, "retained", r.State)
	epoch, e := store.BeginChecked(ctx, uuid.NewString(), uuid.NewString(), func(records map[string]recovery.RetentionRecord) error {
		_, e := backup.CheckRetentionRecoveryCoverage(ctx, db, coremodel.PowerXSchema, records)
		return e
	})
	require.NoError(t, e)
	run("pg_restore", "--clean", "--if-exists", "--exit-on-error", "--no-owner", "--dbname="+dsn, file)
	assertRetainSources(t, db, f, false)
	got, e := jobs.CleanupResolutionRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, r, got)
	_, e = jobs.ResolveCleanupRetained(ctx, f.a.ID, in, func(backup.CleanupResolutionEvent) error {
		t.Fatal("historical operation reprotected current rows")
		return nil
	})
	require.NoError(t, e)
	assertRetainSources(t, db, f, false)
	state, e := s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	finishOwnedRecovery(t, s, apply, epoch.Epoch, state.DesiredRevision)
	fresh := resolutionInput(t, jobs, f)
	require.NotEqual(t, in.ReviewVersion, fresh.ReviewVersion)
	newReceipt, e := jobs.ResolveCleanupRetained(ctx, f.a.ID, fresh, func(backup.CleanupResolutionEvent) error { return nil })
	require.NoError(t, e)
	require.Equal(t, "retained", newReceipt.State)
	assertResolved(t, db, f)
	var old m.BackupCleanupResolution
	require.NoError(t, db.Where("request_id=?", in.RequestID).First(&old).Error)
	require.Equal(t, "checking", old.State, "the restored original SQL history remains unchanged")
}

func TestCleanupResolutionNativeOptionalSchemaDoesNotBreakAdmission(t *testing.T) {
	s, jobs, db, _, _, _, f := resolutionSetup(t)
	ctx := controlRoot()
	beforeAdmission := s.CheckDatabaseAdmission(ctx)
	require.NoError(t, db.Exec("DROP INDEX "+pgx.Identifier{coremodel.PowerXSchema, "uk_backup_resolution_request"}.Sanitize()).Error)
	_, e := jobs.PreviewCleanupResolution(ctx, f.a.ID, f.a.CleanupToken)
	require.Equal(t, "BACKUP_RESOLUTION_SCHEMA_NOT_READY", dto.CodeOf(e))
	_, e = jobs.RetentionRequest(ctx, f.in.RequestID)
	require.NoError(t, e)
	status, e := s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	require.True(t, status.Confirmed)
	require.Equal(t, dto.CodeOf(beforeAdmission), dto.CodeOf(s.CheckDatabaseAdmission(ctx)), "optional receipt schema cannot change ordinary admission")
}
