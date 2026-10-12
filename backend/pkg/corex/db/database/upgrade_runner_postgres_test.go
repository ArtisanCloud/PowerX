package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func upgradeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("POWERX_UPGRADE_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required: POWERX_UPGRADE_TEST_DSN")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	var name string
	require.NoError(t, db.Raw("select current_database()").Scan(&name).Error)
	require.True(t, strings.HasPrefix(name, "powerx_upgrade_test_"), "test database must have a dedicated prefix")
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS public.database_upgrade_runs, public.database_migration_records, public.upgrade_test_values").Error)
	require.NoError(t, db.Exec("CREATE TABLE public.upgrade_test_values (id integer primary key, value integer not null)").Error)
	require.NoError(t, db.Exec("INSERT INTO public.upgrade_test_values VALUES (1,0)").Error)
	sqldb, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqldb.Close() })
	return db
}

func testStep(id string, transactional bool, f func(context.Context, *gorm.DB) error) UpgradeStep {
	h := sha256.Sum256([]byte(id))
	return UpgradeStep{ID: id, Checksum: hex.EncodeToString(h[:]), DependsOn: []string{}, Transactional: transactional, Recovery: "test", Apply: f}
}
func testRunner(db *gorm.DB, steps ...UpgradeStep) *UpgradeRunner {
	return &UpgradeRunner{DB: db, ReleaseID: "test-release", SourceSHA: strings.Repeat("1", 40), MigrationVersion: "test", Steps: steps}
}
func planHash(t *testing.T, r *UpgradeRunner) string {
	t.Helper()
	p, err := r.Plan(context.Background(), "plan")
	require.NoError(t, err)
	require.Empty(t, p.Blockers)
	return p.PlanHash
}

func TestUpgradeReadOnlyAndLegacyJournal(t *testing.T) {
	db := upgradeTestDB(t)
	r := testRunner(db, testStep("one", true, func(context.Context, *gorm.DB) error { return nil }))
	_, err := r.Plan(context.Background(), "status")
	require.NoError(t, err)
	require.False(t, db.Migrator().HasTable(&UpgradeRun{}))
	require.False(t, db.Migrator().HasTable(&databaseMigrationRecord{}))
	require.NoError(t, db.Exec("CREATE TABLE public.database_migration_records (migration_id varchar(128) primary key, applied_at timestamptz not null)").Error)
	require.NoError(t, db.Exec("INSERT INTO public.database_migration_records VALUES ('legacy-iam', '2026-01-01T00:00:00Z')").Error)
	p, err := r.Plan(context.Background(), "plan")
	require.NoError(t, err)
	require.Len(t, p.Pending, 1)
	require.False(t, db.Migrator().HasColumn("public.database_migration_records", "checksum"))
	a, err := r.Apply(context.Background(), p.PlanHash, time.Second)
	require.NoError(t, err)
	require.Equal(t, "succeeded", a.Status)
	var record databaseMigrationRecord
	require.NoError(t, db.Where("migration_id = ?", "legacy-iam").First(&record).Error)
	require.Equal(t, 2026, record.AppliedAt.Year())
	require.Empty(t, record.Checksum)
}

func TestUpgradeTransactionFailureRetryIdempotenceAndChecksum(t *testing.T) {
	db := upgradeTestDB(t)
	fail := true
	one := testStep("one", true, func(_ context.Context, tx *gorm.DB) error {
		return tx.Exec("UPDATE public.upgrade_test_values SET value=value+1").Error
	})
	two := testStep("two", true, func(_ context.Context, tx *gorm.DB) error {
		if err := tx.Exec("UPDATE public.upgrade_test_values SET value=value+10").Error; err != nil {
			return err
		}
		if fail {
			return errors.New("password=TOP_SECRET must not be exposed")
		}
		return nil
	})
	two.DependsOn = []string{"one"}
	r := testRunner(db, one, two)
	a, err := r.Apply(context.Background(), planHash(t, r), time.Second)
	require.NoError(t, err)
	require.Equal(t, "failed", a.Status)
	require.True(t, a.Partial)
	require.True(t, a.Retryable)
	require.False(t, a.RequiresRestore)
	var value int
	require.NoError(t, db.Raw("SELECT value FROM public.upgrade_test_values").Scan(&value).Error)
	require.Equal(t, 1, value)
	var count int64
	require.NoError(t, db.Model(&databaseMigrationRecord{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	encoded, err := json.Marshal(a)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "TOP_SECRET")
	fail = false
	a, err = r.Apply(context.Background(), planHash(t, r), time.Second)
	require.NoError(t, err)
	require.Equal(t, "succeeded", a.Status)
	require.Equal(t, []string{"one"}, a.Skipped)
	a, err = r.Apply(context.Background(), planHash(t, r), time.Second)
	require.NoError(t, err)
	require.Empty(t, a.Applied)
	require.NoError(t, db.Raw("SELECT value FROM public.upgrade_test_values").Scan(&value).Error)
	require.Equal(t, 11, value)
	r.Steps[0].Checksum = strings.Repeat("0", 64)
	p, err := r.Plan(context.Background(), "plan")
	require.NoError(t, err)
	require.Equal(t, "UPGRADE_MIGRATION_CHECKSUM_MISMATCH", p.ErrorCode)
}

func TestUpgradeGlobalLockAndPlanDrift(t *testing.T) {
	db := upgradeTestDB(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan *UpgradeResult)
	r := testRunner(db, testStep("locked", true, func(context.Context, *gorm.DB) error { close(entered); <-release; return nil }))
	hash := planHash(t, r)
	go func() {
		out, err := r.Apply(context.Background(), hash, time.Second)
		if err != nil {
			out.ErrorCode = "test-error"
		}
		done <- out
	}()
	<-entered
	a, err := r.Apply(context.Background(), hash, 0)
	require.NoError(t, err)
	require.Equal(t, "UPGRADE_LOCK_BUSY", a.ErrorCode)
	close(release)
	require.Equal(t, "succeeded", (<-done).Status)
	a, err = r.Apply(context.Background(), hash, 0)
	require.NoError(t, err)
	require.Equal(t, "UPGRADE_PLAN_CHANGED", a.ErrorCode)
	var locks int
	require.NoError(t, db.Raw("SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND database=(SELECT oid FROM pg_database WHERE datname=current_database())").Scan(&locks).Error)
	require.Zero(t, locks)
}

func TestUpgradeNontransactionalFailureRequiresRestore(t *testing.T) {
	db := upgradeTestDB(t)
	r := testRunner(db, testStep("external", false, func(_ context.Context, tx *gorm.DB) error {
		require.NoError(t, tx.Exec("UPDATE public.upgrade_test_values SET value=99").Error)
		return errors.New("external failure")
	}))
	a, err := r.Apply(context.Background(), planHash(t, r), time.Second)
	require.NoError(t, err)
	require.True(t, a.RequiresRestore)
	require.True(t, a.Partial)
	require.False(t, a.Retryable)
	p, err := r.Plan(context.Background(), "plan")
	require.NoError(t, err)
	require.Equal(t, "UPGRADE_RECOVERY_REQUIRED", p.ErrorCode)
	a, err = r.Apply(context.Background(), p.PlanHash, time.Second)
	require.NoError(t, err)
	require.Equal(t, "blocked", a.Status)
}

func TestUpgradeInterruptedTransactionAndVerification(t *testing.T) {
	db := upgradeTestDB(t)
	var calls atomic.Int32
	step := testStep("resume", true, func(_ context.Context, tx *gorm.DB) error {
		calls.Add(1)
		return tx.Exec("UPDATE public.upgrade_test_values SET value=1").Error
	})
	r := testRunner(db, step)
	require.NoError(t, MigrateUpgradeJournal(db))
	old := UpgradeRun{RunUUID: uuid.New(), SourceSHA: r.SourceSHA, ReleaseID: r.ReleaseID, PlanHash: "previous", Status: "running", MigrationID: "resume", StartedAt: time.Now().UTC()}
	require.NoError(t, db.Create(&old).Error)
	a, err := r.Apply(context.Background(), planHash(t, r), time.Second)
	require.NoError(t, err)
	require.Equal(t, "succeeded", a.Status)
	require.NoError(t, db.First(&old, "run_uuid = ?", old.RunUUID).Error)
	require.Equal(t, "interrupted", old.Status)
	require.EqualValues(t, 1, calls.Load())
	step = testStep("verify", true, func(_ context.Context, tx *gorm.DB) error {
		return tx.Exec("UPDATE public.upgrade_test_values SET value=7").Error
	})
	step.Verify = func(context.Context, *gorm.DB) error { return errors.New("postcheck failed") }
	r.Steps = append(r.Steps, step)
	a, err = r.Apply(context.Background(), planHash(t, r), time.Second)
	require.NoError(t, err)
	require.Equal(t, "failed", a.Status)
	var value int
	require.NoError(t, db.Raw("SELECT value FROM public.upgrade_test_values").Scan(&value).Error)
	require.Equal(t, 1, value)
}
