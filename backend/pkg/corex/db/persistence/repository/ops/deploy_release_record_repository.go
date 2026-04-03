package ops

import (
	"context"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type DeployReleaseRecordRepository struct {
	*repository.BaseRepository[modelops.DeployReleaseRecord]
	db *gorm.DB
}

func NewDeployReleaseRecordRepository(db *gorm.DB) *DeployReleaseRecordRepository {
	return &DeployReleaseRecordRepository{
		BaseRepository: repository.NewBaseRepository[modelops.DeployReleaseRecord](db),
		db:             db,
	}
}

func (r *DeployReleaseRecordRepository) FindRunningByEnvironment(ctx context.Context, environment string) (*modelops.DeployReleaseRecord, error) {
	var row modelops.DeployReleaseRecord
	if err := r.db.WithContext(ctx).
		Where("environment = ? AND status = ?", environment, modelops.DeployStatusRunning).
		Order("id DESC").
		Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *DeployReleaseRecordRepository) ListByEnvironment(ctx context.Context, environment string, limit, offset int) ([]modelops.DeployReleaseRecord, int64, error) {
	q := r.db.WithContext(ctx).Model(&modelops.DeployReleaseRecord{})
	if environment != "" {
		q = q.Where("environment = ?", environment)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []modelops.DeployReleaseRecord
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
