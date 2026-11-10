package system_recovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCurrentStorageNativeHistoryGapBlocksBeforeEpochAndSQL(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("owned temporary PostgreSQL only")
	}
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.Host, "/tmp/powerx-control-owned-"))
	require.Equal(t, "storage_control_test", cfg.User)
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	previous := model.PowerXSchema
	schema := "owned_recovery_history_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	model.PowerXSchema = schema
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		model.PowerXSchema = previous
		require.NoError(t, pool.Close())
	})
	require.NoError(t, database.MigrateBackupCleanupRetentionModels(db))
	r, _, _ := currentFixture(t)
	ctx := context.Background()
	j, e := r.Coordinator.Store.EnqueueInstanceRestore(privileged(ctx), currentReadySnapshot(t, r), uuid.NewString(), 1)
	require.NoError(t, e)
	input := backup.CleanupRetainInput{RequestID: uuid.NewString(), ExpectedToken: uuid.NewString(), ReviewVersion: strings.Repeat("a", 64), Reason: "owned historical receipt", Confirm: true}
	raw, e := json.Marshal(struct {
		Artifact uint64
		Input    backup.CleanupRetainInput
	}{1, input})
	require.NoError(t, e)
	sum := sha256.Sum256(raw)
	row := m.BackupCleanupRetention{RequestID: uuid.MustParse(input.RequestID), ArtifactID: 1, ActorID: 9, CleanupToken: input.ExpectedToken, ReviewVersion: input.ReviewVersion, InputDigest: hex.EncodeToString(sum[:]), ScopeFingerprint: strings.Repeat("b", 64), Reason: input.Reason, State: "checking"}
	row.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, db.Create(&row).Error)
	before, e := r.StorageRecoveryAnchor().Read(ctx)
	require.NoError(t, e)
	e = r.beginStorageRestoreChecked(ctx, &j, dsn)
	require.Error(t, e)
	require.Contains(t, e.Error(), "BACKUP_HISTORY_COVERAGE_REQUIRED")
	after, e := r.StorageRecoveryAnchor().Read(ctx)
	require.NoError(t, e)
	require.Equal(t, before, after)
	_, e = r.StorageRecoveryAnchor().ReserveRetention(ctx, recovery.RetentionRecord{RequestID: input.RequestID, ArtifactID: 1, ActorID: 9, Token: input.ExpectedToken, ReviewVersion: input.ReviewVersion, InputDigest: row.InputDigest, Scope: row.ScopeFingerprint, Reason: input.Reason, CreatedAt: row.CreatedAt, State: "checking"})
	require.NoError(t, e)
	require.NoError(t, r.beginStorageRestoreChecked(ctx, &j, dsn))
	found := false
	for _, c := range j.Checks {
		if c.Name == "backup_request_history_coverage" {
			found = true
			require.True(t, c.Passed)
			require.Contains(t, c.Detail, "1 条")
		}
	}
	require.True(t, found)
	var count int64
	require.NoError(t, db.Model(&m.BackupCleanupRetention{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
