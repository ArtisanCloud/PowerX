package eventfabric

import (
	"context"
	"errors"
	"strings"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaskHistoryRepository struct {
	db *gorm.DB
}

func NewTaskHistoryRepository(db *gorm.DB) *TaskHistoryRepository {
	return &TaskHistoryRepository{db: db}
}

func (r *TaskHistoryRepository) FindByKey(ctx context.Context, tenantKey, subscriberID, taskID string) (*eventfabricmodel.TaskHistory, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	tenantKey = strings.TrimSpace(tenantKey)
	subscriberID = strings.TrimSpace(subscriberID)
	taskID = strings.TrimSpace(taskID)
	if tenantKey == "" || subscriberID == "" || taskID == "" {
		return nil, nil
	}
	var record eventfabricmodel.TaskHistory
	err := r.db.WithContext(ctx).
		Model(&eventfabricmodel.TaskHistory{}).
		Where("tenant_key = ? AND subscriber_id = ? AND task_id = ?", tenantKey, subscriberID, taskID).
		Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *TaskHistoryRepository) Save(ctx context.Context, record *eventfabricmodel.TaskHistory) error {
	if r == nil || r.db == nil || record == nil {
		return nil
	}
	record.TaskID = strings.TrimSpace(record.TaskID)
	record.TenantKey = strings.TrimSpace(record.TenantKey)
	record.SubscriberID = strings.TrimSpace(record.SubscriberID)
	if record.TaskID == "" || record.TenantKey == "" || record.SubscriberID == "" {
		return nil
	}
	if record.LastSeenAt.IsZero() {
		record.LastSeenAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "task_id"},
			{Name: "tenant_key"},
			{Name: "subscriber_id"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"topic":         record.Topic,
			"kind":          record.Kind,
			"source":        record.Source,
			"trace_id":      record.TraceID,
			"status":        record.Status,
			"attempt":       record.Attempt,
			"error_message": record.ErrorMessage,
			"payload":       record.Payload,
			"metadata":      record.Metadata,
			"submitted_at":  record.SubmittedAt,
			"started_at":    record.StartedAt,
			"completed_at":  record.CompletedAt,
			"last_seen_at":  record.LastSeenAt,
			"updated_at":    time.Now().UTC(),
		}),
	}).Create(record).Error
}

func (r *TaskHistoryRepository) ListRecent(ctx context.Context, tenantKey, subscriberID string, limit int) ([]*eventfabricmodel.TaskHistory, error) {
	if r == nil || r.db == nil {
		return []*eventfabricmodel.TaskHistory{}, nil
	}
	tenantKey = strings.TrimSpace(tenantKey)
	subscriberID = strings.TrimSpace(subscriberID)
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	query := r.db.WithContext(ctx).Model(&eventfabricmodel.TaskHistory{})
	if tenantKey != "" {
		query = query.Where("tenant_key = ?", tenantKey)
	}
	if subscriberID != "" {
		query = query.Where("subscriber_id = ?", subscriberID)
	}
	var records []*eventfabricmodel.TaskHistory
	if err := query.Order("last_seen_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *TaskHistoryRepository) ListPage(ctx context.Context, tenantKey, subscriberID string, page, pageSize int) ([]*eventfabricmodel.TaskHistory, int64, error) {
	if r == nil || r.db == nil {
		return []*eventfabricmodel.TaskHistory{}, 0, nil
	}
	tenantKey = strings.TrimSpace(tenantKey)
	subscriberID = strings.TrimSpace(subscriberID)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	query := r.db.WithContext(ctx).Model(&eventfabricmodel.TaskHistory{})
	if tenantKey != "" {
		query = query.Where("tenant_key = ?", tenantKey)
	}
	if subscriberID != "" {
		query = query.Where("subscriber_id = ?", subscriberID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []*eventfabricmodel.TaskHistory
	if err := query.Order("last_seen_at DESC, submitted_at DESC, task_id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}
