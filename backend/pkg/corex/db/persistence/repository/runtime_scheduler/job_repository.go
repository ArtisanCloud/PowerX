package runtimescheduler

import (
	"context"
	"errors"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	baserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type JobFilter struct {
	TenantUUID string
	OwnerType  string
	OwnerID    string
	Status     string
	Page       int
	PageSize   int
}

type JobRepository struct {
	base *baserepo.BaseRepository[models.SchedulerJob]
	db   *gorm.DB
}

func NewJobRepository(db *gorm.DB) *JobRepository {
	return &JobRepository{base: baserepo.NewBaseRepository[models.SchedulerJob](db), db: db}
}

func (r *JobRepository) Create(ctx context.Context, job *models.SchedulerJob) (*models.SchedulerJob, error) {
	return r.base.Create(ctx, job)
}

func (r *JobRepository) Update(ctx context.Context, job *models.SchedulerJob) (*models.SchedulerJob, error) {
	return r.base.Update(ctx, job)
}

func (r *JobRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*models.SchedulerJob, error) {
	var row models.SchedulerJob
	err := r.db.WithContext(ctx).Where("uuid = ?", id).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *JobRepository) List(ctx context.Context, filter JobFilter) ([]*models.SchedulerJob, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.SchedulerJob{})
	if v := strings.TrimSpace(filter.TenantUUID); v != "" {
		query = query.Where("tenant_uuid = ?", v)
	}
	if v := strings.TrimSpace(filter.OwnerType); v != "" {
		query = query.Where("owner_type = ?", v)
	}
	if v := strings.TrimSpace(filter.OwnerID); v != "" {
		query = query.Where("owner_id = ?", v)
	}
	if v := strings.TrimSpace(filter.Status); v != "" {
		query = query.Where("status = ?", v)
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
	var rows []*models.SchedulerJob
	if err := query.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *JobRepository) ListDue(ctx context.Context, now time.Time, limit int) ([]*models.SchedulerJob, error) {
	query := r.db.WithContext(ctx).
		Where("status = ?", models.JobStatusActive).
		Where("next_run_at IS NOT NULL AND next_run_at <= ?", now.UTC())
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []*models.SchedulerJob
	if err := query.Order("next_run_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *JobRepository) ClaimDue(ctx context.Context, id uuid.UUID, dueAt time.Time) (*models.SchedulerJob, error) {
	var row models.SchedulerJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("uuid = ?", id).
			Take(&row).Error; err != nil {
			return err
		}
		if row.Status != models.JobStatusActive || row.NextRunAt == nil || row.NextRunAt.After(dueAt.UTC()) {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *JobRepository) UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&models.SchedulerJob{}).
		Where("uuid = ?", id).
		Updates(fields).Error
}
