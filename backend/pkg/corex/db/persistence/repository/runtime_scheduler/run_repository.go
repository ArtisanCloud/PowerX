package runtimescheduler

import (
	"context"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	baserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RunFilter struct {
	JobUUID  uuid.UUID
	Page     int
	PageSize int
}

type RunRepository struct {
	base *baserepo.BaseRepository[models.SchedulerJobRun]
	db   *gorm.DB
}

func NewRunRepository(db *gorm.DB) *RunRepository {
	return &RunRepository{base: baserepo.NewBaseRepository[models.SchedulerJobRun](db), db: db}
}

func (r *RunRepository) Create(ctx context.Context, run *models.SchedulerJobRun) (*models.SchedulerJobRun, error) {
	return r.base.Create(ctx, run)
}

func (r *RunRepository) List(ctx context.Context, filter RunFilter) ([]*models.SchedulerJobRun, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.SchedulerJobRun{})
	if filter.JobUUID != uuid.Nil {
		query = query.Where("job_uuid = ?", filter.JobUUID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if filter.PageSize > 0 {
		query = query.Limit(filter.PageSize)
	}
	if filter.Page > 0 && filter.PageSize > 0 {
		query = query.Offset((filter.Page - 1) * filter.PageSize)
	}
	var rows []*models.SchedulerJobRun
	if err := query.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
