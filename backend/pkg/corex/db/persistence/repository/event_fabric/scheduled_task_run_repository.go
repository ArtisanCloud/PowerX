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

type ScheduledTaskRunFilter struct {
	TenantUUID        string
	ScheduledTaskUUID uuid.UUID
	Statuses          []string
	TriggerTypes      []string
	TraceID           string
	Page              int
	PageSize          int
}

// ScheduledTaskRunRepository 管理定时任务执行记录。
type ScheduledTaskRunRepository struct {
	base *baserepo.BaseRepository[eventfabricmodel.ScheduledTaskRun]
	db   *gorm.DB
}

func NewScheduledTaskRunRepository(db *gorm.DB) *ScheduledTaskRunRepository {
	return &ScheduledTaskRunRepository{
		base: baserepo.NewBaseRepository[eventfabricmodel.ScheduledTaskRun](db),
		db:   db,
	}
}

func (r *ScheduledTaskRunRepository) WithDB(db *gorm.DB) *ScheduledTaskRunRepository {
	if db == nil {
		return r
	}
	return NewScheduledTaskRunRepository(db)
}

func (r *ScheduledTaskRunRepository) Create(ctx context.Context, run *eventfabricmodel.ScheduledTaskRun) (*eventfabricmodel.ScheduledTaskRun, error) {
	return r.base.Create(ctx, run)
}

func (r *ScheduledTaskRunRepository) Update(ctx context.Context, run *eventfabricmodel.ScheduledTaskRun) (*eventfabricmodel.ScheduledTaskRun, error) {
	return r.base.Update(ctx, run)
}

func (r *ScheduledTaskRunRepository) FindByUUID(ctx context.Context, runUUID uuid.UUID) (*eventfabricmodel.ScheduledTaskRun, error) {
	var record eventfabricmodel.ScheduledTaskRun
	err := r.db.WithContext(ctx).Where("uuid = ?", runUUID).Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *ScheduledTaskRunRepository) List(ctx context.Context, filter ScheduledTaskRunFilter) ([]*eventfabricmodel.ScheduledTaskRun, int64, error) {
	query := r.db.WithContext(ctx).Model(&eventfabricmodel.ScheduledTaskRun{})

	if tenantUUID := strings.TrimSpace(filter.TenantUUID); tenantUUID != "" {
		query = query.Where("tenant_uuid = ?", tenantUUID)
	}
	if filter.ScheduledTaskUUID != uuid.Nil {
		query = query.Where("scheduled_task_uuid = ?", filter.ScheduledTaskUUID)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	if len(filter.TriggerTypes) > 0 {
		query = query.Where("trigger_type IN ?", filter.TriggerTypes)
	}
	if traceID := strings.TrimSpace(filter.TraceID); traceID != "" {
		query = query.Where("trace_id = ?", traceID)
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

	var records []*eventfabricmodel.ScheduledTaskRun
	if err := query.Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func (r *ScheduledTaskRunRepository) UpdateFields(ctx context.Context, runUUID uuid.UUID, fields map[string]interface{}) error {
	if runUUID == uuid.Nil {
		return errors.New("scheduled task run uuid is required")
	}
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&eventfabricmodel.ScheduledTaskRun{}).
		Where("uuid = ?", runUUID).
		Updates(fields).Error
}

func (r *ScheduledTaskRunRepository) MarkRunning(ctx context.Context, runUUID uuid.UUID, startedAt time.Time) error {
	return r.UpdateFields(ctx, runUUID, map[string]interface{}{
		"status":     eventfabricmodel.ScheduledTaskRunStatusRunning,
		"started_at": startedAt.UTC(),
	})
}

func (r *ScheduledTaskRunRepository) MarkFinished(ctx context.Context, runUUID uuid.UUID, status, errorMessage string, finishedAt time.Time) error {
	updates := map[string]interface{}{
		"status":      status,
		"finished_at": finishedAt.UTC(),
	}
	if strings.TrimSpace(errorMessage) != "" {
		updates["error_message"] = errorMessage
	}
	return r.UpdateFields(ctx, runUUID, updates)
}
