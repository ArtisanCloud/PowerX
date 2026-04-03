package ops

import (
	"context"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type RestoreDrillRecordRepository struct {
	*repository.BaseRepository[modelops.RestoreDrillRecord]
	db *gorm.DB
}

func NewRestoreDrillRecordRepository(db *gorm.DB) *RestoreDrillRecordRepository {
	return &RestoreDrillRecordRepository{
		BaseRepository: repository.NewBaseRepository[modelops.RestoreDrillRecord](db),
		db:             db,
	}
}

func (r *RestoreDrillRecordRepository) ListBySourceJobID(ctx context.Context, sourceJobID uint64, limit, offset int) ([]modelops.RestoreDrillRecord, int64, error) {
	q := r.db.WithContext(ctx).Model(&modelops.RestoreDrillRecord{})
	if sourceJobID > 0 {
		q = q.Where("source_job_id = ?", sourceJobID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []modelops.RestoreDrillRecord
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
