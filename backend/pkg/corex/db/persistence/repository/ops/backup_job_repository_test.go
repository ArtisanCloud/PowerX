package ops

import (
	"context"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBackupJobViewsFilterBeforePaginationAndKeepExecutionHistory(t *testing.T) {
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	require.NoError(t, db.AutoMigrate(&modelops.BackupJob{}, &modelops.BackupArtifact{}))
	// 较新的无文件／失败／已清理任务不能挤占可恢复列表的第一页。
	for _, row := range []modelops.BackupJob{
		{PowerUUIDModel: coremodel.PowerUUIDModel{ID: 1}, PolicyID: 1, Status: "success"},
		{PowerUUIDModel: coremodel.PowerUUIDModel{ID: 2}, PolicyID: 1, Status: "success"},
		{PowerUUIDModel: coremodel.PowerUUIDModel{ID: 3}, PolicyID: 1, Status: "success"},
		{PowerUUIDModel: coremodel.PowerUUIDModel{ID: 4}, PolicyID: 1, Status: "failed"},
		{PowerUUIDModel: coremodel.PowerUUIDModel{ID: 5}, PolicyID: 1, Status: "success"},
		{PowerUUIDModel: coremodel.PowerUUIDModel{ID: 6}, PolicyID: 2, Status: "success"},
	} {
		require.NoError(t, db.Create(&row).Error)
	}
	for _, artifact := range []modelops.BackupArtifact{{JobID: 1, StorageURI: "file:///backup/1.dump"}, {JobID: 1, StorageURI: "file:///backup/1-second.dump"}, {JobID: 2, StorageURI: "file:///backup/2.dump"}, {JobID: 3, StorageURI: "file:///backup/3.dump"}, {JobID: 4, StorageURI: "file:///backup/failed.dump"}, {JobID: 5, StorageURI: "file:///backup/stale-registration.dump"}, {JobID: 5, StorageURI: "  "}, {JobID: 6, StorageURI: "file:///backup/6.dump"}} {
		require.NoError(t, db.Create(&artifact).Error)
	}
	require.NoError(t, db.Where("job_id = ?", 3).Delete(&modelops.BackupArtifact{}).Error)
	repo := NewBackupJobRepository(db)
	ctx := context.Background()
	rows, total, e := repo.ListWithView(ctx, 1, "", nil, nil, "available", 1, 0)
	require.NoError(t, e)
	require.EqualValues(t, 2, total)
	require.Len(t, rows, 1)
	require.EqualValues(t, 2, rows[0].ID)
	rows, total, e = repo.ListWithView(ctx, 1, "", nil, nil, "available", 1, 1)
	require.NoError(t, e)
	require.EqualValues(t, 2, total)
	require.EqualValues(t, 1, rows[0].ID)
	_, total, e = repo.ListWithView(ctx, 0, "", nil, nil, "available", 20, 0)
	require.NoError(t, e)
	require.EqualValues(t, 3, total)
	_, total, e = repo.ListWithView(ctx, 0, "failed", nil, nil, "available", 20, 0)
	require.NoError(t, e)
	require.Zero(t, total)
	for _, view := range []string{"", "history"} {
		rows, total, e = repo.ListWithView(ctx, 1, "", nil, nil, view, 20, 0)
		require.NoError(t, e)
		require.EqualValues(t, 5, total)
		require.Len(t, rows, 5)
	}
}
