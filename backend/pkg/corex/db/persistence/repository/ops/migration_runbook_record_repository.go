package ops

import (
	"context"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type MigrationRunbookRecordRepository struct {
	*repository.BaseRepository[modelops.MigrationRunbookRecord]
	db *gorm.DB
}

func NewMigrationRunbookRecordRepository(db *gorm.DB) *MigrationRunbookRecordRepository {
	return &MigrationRunbookRecordRepository{
		BaseRepository: repository.NewBaseRepository[modelops.MigrationRunbookRecord](db),
		db:             db,
	}
}

func (r *MigrationRunbookRecordRepository) List(ctx context.Context, sourceEnv, targetEnv string, limit, offset int) ([]modelops.MigrationRunbookRecord, int64, error) {
	q := r.db.WithContext(ctx).Model(&modelops.MigrationRunbookRecord{})
	if sourceEnv != "" {
		q = q.Where("source_env = ?", sourceEnv)
	}
	if targetEnv != "" {
		q = q.Where("target_env = ?", targetEnv)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []modelops.MigrationRunbookRecord
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
