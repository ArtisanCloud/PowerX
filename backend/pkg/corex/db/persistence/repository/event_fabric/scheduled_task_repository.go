package eventfabric

import (
	"context"
	"errors"
	"strings"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	baserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ScheduledTaskFilter struct {
	TenantUUID string
	Statuses   []string
	JobKey     string
	Kind       string
	Page       int
	PageSize   int
}

// ScheduledTaskRepository 管理定时任务定义。
type ScheduledTaskRepository struct {
	base *baserepo.BaseRepository[eventfabricmodel.ScheduledTask]
	db   *gorm.DB
}

func NewScheduledTaskRepository(db *gorm.DB) *ScheduledTaskRepository {
	return &ScheduledTaskRepository{
		base: baserepo.NewBaseRepository[eventfabricmodel.ScheduledTask](db),
		db:   db,
	}
}

func (r *ScheduledTaskRepository) WithDB(db *gorm.DB) *ScheduledTaskRepository {
	if db == nil {
		return r
	}
	return NewScheduledTaskRepository(db)
}

func (r *ScheduledTaskRepository) Create(ctx context.Context, task *eventfabricmodel.ScheduledTask) (*eventfabricmodel.ScheduledTask, error) {
	return r.base.Create(ctx, task)
}

func (r *ScheduledTaskRepository) Update(ctx context.Context, task *eventfabricmodel.ScheduledTask) (*eventfabricmodel.ScheduledTask, error) {
	return r.base.Update(ctx, task)
}

func (r *ScheduledTaskRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.ScheduledTask, error) {
	var record eventfabricmodel.ScheduledTask
	err := r.db.WithContext(ctx).Where("uuid = ?", id).Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *ScheduledTaskRepository) FindByTenantAndJobKey(ctx context.Context, tenantUUID, jobKey string) (*eventfabricmodel.ScheduledTask, error) {
	tenantUUID = strings.TrimSpace(tenantUUID)
	jobKey = strings.TrimSpace(jobKey)
	if tenantUUID == "" || jobKey == "" {
		return nil, nil
	}

	var record eventfabricmodel.ScheduledTask
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND job_key = ?", tenantUUID, jobKey).
		Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *ScheduledTaskRepository) List(ctx context.Context, filter ScheduledTaskFilter) ([]*eventfabricmodel.ScheduledTask, int64, error) {
	query := r.db.WithContext(ctx).Model(&eventfabricmodel.ScheduledTask{})

	if tenantUUID := strings.TrimSpace(filter.TenantUUID); tenantUUID != "" {
		query = query.Where("tenant_uuid = ?", tenantUUID)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	if jobKey := strings.TrimSpace(filter.JobKey); jobKey != "" {
		query = query.Where("job_key = ?", jobKey)
	}
	if kind := strings.TrimSpace(filter.Kind); kind != "" {
		query = query.Where("kind = ?", kind)
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

	var records []*eventfabricmodel.ScheduledTask
	if err := query.Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func (r *ScheduledTaskRepository) ListDueTasks(ctx context.Context, now time.Time, tenantUUID string, limit int) ([]*eventfabricmodel.ScheduledTask, error) {
	query := r.db.WithContext(ctx).
		Where("status = ?", eventfabricmodel.ScheduledTaskStatusEnabled).
		Where("next_run_at IS NOT NULL AND next_run_at <= ?", now.UTC())
	if scopedTenant := strings.TrimSpace(tenantUUID); scopedTenant != "" {
		query = query.Where("tenant_uuid = ?", scopedTenant)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	var records []*eventfabricmodel.ScheduledTask
	if err := query.Order("next_run_at ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *ScheduledTaskRepository) UpdateFields(ctx context.Context, taskUUID uuid.UUID, fields map[string]interface{}) error {
	if taskUUID == uuid.Nil {
		return errors.New("scheduled task uuid is required")
	}
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&eventfabricmodel.ScheduledTask{}).
		Where("uuid = ?", taskUUID).
		Updates(fields).Error
}
