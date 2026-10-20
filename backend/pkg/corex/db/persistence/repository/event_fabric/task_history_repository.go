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

// Register 使用统一任务历史作为提交记录，可与业务事务一起提交；重复提交不覆盖终态。
func (r *TaskHistoryRepository) Register(ctx context.Context, tx *gorm.DB, record *eventfabricmodel.TaskHistory) error {
	if tx == nil {
		tx = r.db
	}
	if tx == nil {
		return errors.New("task history unavailable")
	}
	return tx.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "task_id"}, {Name: "tenant_key"}, {Name: "subscriber_id"}}, DoNothing: true}).Create(record).Error
}

// WithRecord 在共享任务行锁内校验投递归属，并允许业务结果与任务完成原子提交。
func (r *TaskHistoryRepository) WithRecord(ctx context.Context, tenant, subscriber, id string, apply func(*gorm.DB, *eventfabricmodel.TaskHistory) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row eventfabricmodel.TaskHistory
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_key = ? AND subscriber_id = ? AND task_id = ?", tenant, subscriber, id).First(&row).Error; err != nil {
			return err
		}
		if err := apply(tx, &row); err != nil {
			return err
		}
		row.LastSeenAt = time.Now().UTC()
		return tx.Save(&row).Error
	})
}

// Recoverable 只供统一任务驱动启动补投使用，不是业务模块的私有轮询队列。
func (r *TaskHistoryRepository) Recoverable(ctx context.Context, subscriber string, after uint64, limit int) ([]eventfabricmodel.TaskHistory, error) {
	var rows []eventfabricmodel.TaskHistory
	err := r.db.WithContext(ctx).Where("subscriber_id = ? AND id > ? AND status IN ?", subscriber, after, []string{eventfabricmodel.TaskHistoryStatusPending, eventfabricmodel.TaskHistoryStatusDeferred, eventfabricmodel.TaskHistoryStatusRunning}).Order("id").Limit(limit).Find(&rows).Error
	return rows, err
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
