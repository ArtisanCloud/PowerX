package storagegovrepo

import (
	"context"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"time"
)

type CleanupRepository struct {
	*repository.BaseRepository[m.CleanupRun]
}

func NewCleanupRepository(db *gorm.DB) *CleanupRepository {
	return &CleanupRepository{repository.NewBaseRepository[m.CleanupRun](db)}
}
func (r *CleanupRepository) CreateRun(ctx context.Context, row *m.CleanupRun) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 仅回收终态/过期预览元数据；未确认的删除意图永远不自动移除。
		if err := tx.Unscoped().Where("tenant_id = ? AND instance_id = ? AND state IN ? AND created_at < ?", 0, row.InstanceID, []string{"preview", "completed", "partial"}, time.Now().Add(-30*24*time.Hour)).Delete(&m.CleanupRun{}).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&m.CleanupRun{}).Where("tenant_id = ? AND instance_id = ?", 0, row.InstanceID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 2000 {
			return ErrRevisionConflict
		}
		return tx.Create(row).Error
	})
}
func (r *CleanupRepository) GetRun(ctx context.Context, instance, id string) (*m.CleanupRun, error) {
	var row m.CleanupRun
	err := r.DB.WithContext(ctx).Where("tenant_id = ? AND instance_id = ? AND run_id = ?", 0, instance, id).First(&row).Error
	return &row, err
}
func (r *CleanupRepository) UpdateRun(ctx context.Context, row *m.CleanupRun, expected int64) error {
	out := r.DB.WithContext(ctx).Model(&m.CleanupRun{}).Where("tenant_id = ? AND instance_id = ? AND run_id = ? AND version = ?", 0, row.InstanceID, row.RunID, expected).Updates(map[string]any{"document": row.Document, "state": row.State, "version": row.Version})
	if out.Error != nil {
		return out.Error
	}
	if out.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}
func (r *CleanupRepository) ListRuns(ctx context.Context, instance string) ([]m.CleanupRun, error) {
	var rows []m.CleanupRun
	err := r.DB.WithContext(ctx).Where("tenant_id = ? AND instance_id = ?", 0, instance).Order("id DESC").Limit(100).Find(&rows).Error
	return rows, err
}
