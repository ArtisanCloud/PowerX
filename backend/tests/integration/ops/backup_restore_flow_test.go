package opsintegration

import (
	"context"
	"testing"

	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// SQLite 不能模拟 PostgreSQL 归档恢复；真实文件和隔离库测试在 backup_ops/integration_test.go。
func TestBackupRejectsNonPostgreSQL(t *testing.T) {
	db := setupBackupDB(t)
	jobSvc := backupops.NewJobService(db)
	_, err := jobSvc.TriggerJob(context.Background(), backupops.TriggerJobRequest{PolicyID: 1})
	require.ErrorIs(t, err, backupops.ErrUnsupportedBackupDatabase)
}

func setupBackupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(&modelops.BackupPolicy{}, &modelops.BackupJob{}, &modelops.RestoreDrillRecord{}))
	return db
}
