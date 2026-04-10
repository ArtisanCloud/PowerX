package ops

import (
	"context"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type BackupJobRepository struct {
	*repository.BaseRepository[modelops.BackupJob]
	db *gorm.DB
}

func NewBackupJobRepository(db *gorm.DB) *BackupJobRepository {
	return &BackupJobRepository{
		BaseRepository: repository.NewBaseRepository[modelops.BackupJob](db),
		db:             db,
	}
}

func (r *BackupJobRepository) List(ctx context.Context, policyID uint64, limit, offset int) ([]modelops.BackupJob, int64, error) {
	q := r.db.WithContext(ctx).Model(&modelops.BackupJob{})
	if policyID > 0 {
		q = q.Where("policy_id = ?", policyID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []modelops.BackupJob
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *BackupJobRepository) ExistsRunningByPolicy(ctx context.Context, policyID uint64) (bool, error) {
	if policyID == 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&modelops.BackupJob{}).
		Where("policy_id = ? AND status = ?", policyID, modelops.BackupJobStatusRunning).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
