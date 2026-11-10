package backup_ops

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"strings"
	"testing"
)

func TestCleanupVerificationNativeLockAndSnapshot(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("owned PostgreSQL only")
	}
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/powerx-control-owned-"))
	require.Equal(t, "storage_control_test", cfg.User)
	require.Equal(t, "postgres", cfg.Database)
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	defer pool.Close()
	schema := "backup_verify_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	previous := model.PowerXSchema
	model.PowerXSchema = schema
	defer func() { db.Exec("DROP SCHEMA " + schema + " CASCADE"); model.PowerXSchema = previous }()
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	require.NoError(t, database.MigrateBackupPolicyControlModels(db))
	root, row, _, _ := verifiedReviewFixture(t, 2<<20)
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", root)
	job := m.BackupJob{PolicyID: row.PolicyID, Status: m.BackupJobStatusSuccess, TriggerType: m.BackupTriggerTypeManual, Operator: "owned"}
	job.ID = row.JobID
	require.NoError(t, db.Create(&job).Error)
	a := m.BackupArtifact{JobID: job.ID, StorageURI: row.StorageURI, SizeBytes: row.SizeBytes, Checksum: row.Checksum, CleanupToken: row.CleanupToken, CleanupState: row.CleanupState}
	a.ID = row.ID
	require.NoError(t, db.Create(&a).Error)
	s := NewJobService(db)
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
	pool.SetMaxOpenConns(1)
	_, e = s.VerifyCleanupReview(ctx, row.ID, row.CleanupToken, func(CleanupVerification) error { t.Error("insufficient pool started read"); return nil })
	require.ErrorContains(t, e, "BACKUP_REVIEW_DATABASE_POOL_INSUFFICIENT")
	pool.SetMaxOpenConns(10)
	unlock, e := lockBackupPolicy(ctx, db, row.PolicyID)
	require.NoError(t, e)
	_, e = s.VerifyCleanupReview(ctx, row.ID, row.CleanupToken, func(CleanupVerification) error { t.Error("busy verification started"); return nil })
	require.ErrorContains(t, e, "BACKUP_REVIEW_BUSY")
	unlock()
	out, e := s.VerifyCleanupReview(ctx, row.ID, row.CleanupToken, func(CleanupVerification) error { return nil })
	require.NoError(t, e)
	require.True(t, out.ContentVerified)

	_, e = s.VerifyCleanupReview(ctx, row.ID, row.CleanupToken, func(CleanupVerification) error { return context.Canceled })
	require.ErrorIs(t, e, context.Canceled)
	out, e = s.VerifyCleanupReview(ctx, row.ID, row.CleanupToken, func(CleanupVerification) error { return nil })
	require.NoError(t, e)
	require.True(t, out.ContentVerified)
	_, e = s.VerifyCleanupReview(ctx, row.ID, uuid.NewString(), func(CleanupVerification) error { return nil })
	require.ErrorContains(t, e, "BACKUP_REVIEW_INTENT_CHANGED")
	out, e = s.VerifyCleanupReview(ctx, row.ID, row.CleanupToken, func(CleanupVerification) error { return db.Model(&a).Update("cleanup_token", uuid.NewString()).Error })
	require.NoError(t, e)
	require.Equal(t, "changed", out.State)
	require.False(t, out.ContentVerified)
	require.NoError(t, db.First(&a, a.ID).Error)
	require.Equal(t, "unconfirmed", a.CleanupState)
}
