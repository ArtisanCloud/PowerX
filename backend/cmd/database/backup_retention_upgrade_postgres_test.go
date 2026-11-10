package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBackupRetentionInstalledCoreUpgradeNativePreservesHistoryAndProtection(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("requires owned PostgreSQL")
	}
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/powerx-control-owned-"))
	require.Equal(t, "storage_control_test", cfg.User)
	require.Equal(t, "postgres", cfg.Database)
	admin, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	adminPool, e := admin.DB()
	require.NoError(t, e)
	previous := coremodel.PowerXSchema
	name := "powerx_upgrade_test_retention_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	// The installed core callback includes historical public-qualified SQL. Use
	// a separate owned database, as installed-upgrade acceptance does, not a
	// different search_path that changes the migration's actual target contract.
	coremodel.PowerXSchema = "public"
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() {
		require.NoError(t, admin.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error)
		coremodel.PowerXSchema = previous
		require.NoError(t, adminPool.Close())
	})
	db, e := gorm.Open(postgres.Open(dsn+" dbname="+name), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() {
		require.NoError(t, pool.Close())
	})
	require.NoError(t, database.MigrateBackupCleanupRetentionModels(db))
	require.NoError(t, database.MigrateBackupPolicyControlModels(db))
	job := m.BackupJob{PolicyID: 1, Status: m.BackupJobStatusSuccess, Protected: true, TriggerType: m.BackupTriggerTypeManual}
	require.NoError(t, db.Create(&job).Error)
	a := m.BackupArtifact{JobID: job.ID, CleanupState: "unconfirmed", CleanupToken: uuid.NewString(), StorageURI: "file:///owned/never-opened.dump"}
	require.NoError(t, db.Create(&a).Error)
	now := time.Now().UTC()
	rows := []m.BackupCleanupRetention{}
	for _, state := range []string{"retained", "checking", "rejected"} {
		r := m.BackupCleanupRetention{RequestID: uuid.New(), ArtifactID: a.ID, JobID: job.ID, PolicyID: 1, CleanupToken: a.CleanupToken, ActorID: 1, InputDigest: strings.Repeat("a", 64), ReviewVersion: strings.Repeat("b", 64), ScopeFingerprint: strings.Repeat("c", 64), Reason: "owned installed upgrade", State: state}
		if state != "checking" {
			r.CompletedAt = &now
		}
		require.NoError(t, db.Create(&r).Error)
		// Freeze the actual SQL timestamp precision, rather than a Go nanosecond fixture.
		require.NoError(t, db.Where("request_id=?", r.RequestID).First(&r).Error)
		rows = append(rows, r)
	}
	// 模拟真实 v6 表：新列不存在，现存 retain_cleanup UUID 仍须保留。
	require.NoError(t, database.MigrateBackupCleanupResolutionModels(db))
	oldResolution := m.BackupCleanupResolution{RequestID: uuid.New(), ArtifactID: a.ID, JobID: job.ID, PolicyID: 1, CleanupToken: a.CleanupToken, ActorID: 1, InputDigest: strings.Repeat("a", 64), ReviewVersion: strings.Repeat("b", 64), HistoryDigest: strings.Repeat("d", 64), ScopeFingerprint: strings.Repeat("c", 64), Generation: strings.Repeat("e", 64), Fence: strings.Repeat("f", 64), Reason: "owned v6 disposition", State: "checking"}
	require.NoError(t, db.Create(&oldResolution).Error)
	for _, col := range []string{"operation", "expected_diagnosis", "diagnostic_digest"} {
		require.NoError(t, db.Migrator().DropColumn(&m.BackupCleanupResolution{}, col))
	}
	steps, version, e := installedUpgradeSteps(&config.Config{})
	require.NoError(t, e)
	core := steps[0]
	require.True(t, strings.HasPrefix(core.ID, "20261011_core_"))
	applyCore := core.Apply
	core.Apply = func(ctx context.Context, tx *gorm.DB) error {
		e := applyCore(ctx, tx)
		if e == nil && (!tx.Migrator().HasTable(&m.BackupCleanupResolution{}) || !tx.Migrator().HasColumn(&m.BackupCleanupResolution{}, "operation") || !tx.Migrator().HasColumn(&m.BackupCleanupResolution{}, "diagnostic_digest")) {
			return errors.New("new cleanup resolution audit table missing from installed core callback")
		}
		if e != nil {
			t.Logf("owned core callback failed: %v", e)
		}
		return e
	}
	runner := &database.UpgradeRunner{DB: db, SourceSHA: strings.Repeat("1", 40), ReleaseID: "retention-joint-validation", MigrationVersion: version, Steps: []database.UpgradeStep{core}}
	ctx := context.Background()
	assertPreserved := func() {
		t.Helper()
		for _, before := range rows {
			var after m.BackupCleanupRetention
			require.NoError(t, db.Where("request_id=?", before.RequestID).First(&after).Error)
			require.Equal(t, before, after)
		}
		if db.Migrator().HasColumn(&m.BackupCleanupResolution{}, "operation") {
			var previousResolution m.BackupCleanupResolution
			require.NoError(t, db.Where("request_id=?", oldResolution.RequestID).First(&previousResolution).Error)
			require.Equal(t, "retain_cleanup", previousResolution.Operation)
			require.Empty(t, previousResolution.ExpectedDiagnosis)
			require.Empty(t, previousResolution.DiagnosticDigest)
			require.Equal(t, oldResolution.RequestID, previousResolution.RequestID)
			require.Equal(t, oldResolution.CleanupToken, previousResolution.CleanupToken)
			require.Equal(t, "checking", previousResolution.State)
		}
		var current m.BackupJob
		require.NoError(t, db.First(&current, job.ID).Error)
		require.True(t, current.Protected)
		var artifact m.BackupArtifact
		require.NoError(t, db.First(&artifact, a.ID).Error)
		require.Equal(t, a.CleanupToken, artifact.CleanupToken)
		require.Equal(t, "unconfirmed", artifact.CleanupState)
	}
	plan, e := runner.Plan(ctx, "plan")
	require.NoError(t, e)
	require.Empty(t, plan.Blockers)
	assertPreserved()
	out, e := runner.Apply(ctx, plan.PlanHash, time.Second)
	require.NoError(t, e)
	require.Equal(t, "succeeded", out.Status, "%+v", out)
	assertPreserved()
	plan, e = runner.Plan(ctx, "plan")
	require.NoError(t, e)
	require.Empty(t, plan.Pending)
	_, e = runner.Apply(ctx, plan.PlanHash, time.Second)
	require.NoError(t, e)
	assertPreserved()
	// An injected transactional step exercises the actual runner's compensation
	// boundary. This is not an additional production registry payload.
	fail := true
	checksum := strings.Repeat("d", 64)
	fault := database.UpgradeStep{ID: "owned_transient_fault_" + checksum, Checksum: checksum, DependsOn: []string{core.ID}, Transactional: true, Recovery: "retry_after_new_plan", Apply: func(ctx context.Context, tx *gorm.DB) error {
		if !fail {
			return nil
		}
		if e := tx.WithContext(ctx).Model(&m.BackupJob{}).Where("id=?", job.ID).Update("protected", false).Error; e != nil {
			return e
		}
		if e := tx.WithContext(ctx).Unscoped().Where("request_id=?", rows[0].RequestID).Delete(&m.BackupCleanupRetention{}).Error; e != nil {
			return e
		}
		return errors.New("owned injected migration failure")
	}}
	runner.Steps = append(runner.Steps, fault)
	plan, e = runner.Plan(ctx, "plan")
	require.NoError(t, e)
	out, e = runner.Apply(ctx, plan.PlanHash, time.Second)
	require.NoError(t, e)
	require.Equal(t, "failed", out.Status)
	assertPreserved()
	fail = false
	plan, e = runner.Plan(ctx, "plan")
	require.NoError(t, e)
	out, e = runner.Apply(ctx, plan.PlanHash, time.Second)
	require.NoError(t, e)
	require.Equal(t, "succeeded", out.Status)
	assertPreserved()
	t.Log("official installed core callback + UpgradeRunner plan/apply/repeat/failure/retry preserved all three UUID states, protection and cleanup token")
}
