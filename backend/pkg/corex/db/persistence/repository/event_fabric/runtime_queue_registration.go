package eventfabric

import (
	"context"
	"errors"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRuntimeQueueConflict = errors.New("TASK_QUEUE_IDEMPOTENCY_CONFLICT")

// RegisterRuntime 同时约束消息编号和幂等键，原始信封不会被重复提交覆盖。
func (r *TaskHistoryRepository) RegisterRuntime(ctx context.Context, record *m.TaskHistory) (*m.TaskHistory, error) {
	if r == nil || r.db == nil || record == nil || record.IdempotencyKey == nil || *record.IdempotencyKey == "" {
		return nil, errors.New("TASK_QUEUE_INVALID_ARGUMENT")
	}
	var saved m.TaskHistory
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error; err != nil {
			return err
		}
		var rows []m.TaskHistory
		if err := tx.Where("tenant_key = ? AND subscriber_id = ? AND (task_id = ? OR idempotency_key = ?)", record.TenantKey, record.SubscriberID, record.TaskID, *record.IdempotencyKey).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != 1 {
			return ErrRuntimeQueueConflict
		}
		saved = rows[0]
		if saved.TaskID != record.TaskID || saved.IdempotencyKey == nil || *saved.IdempotencyKey != *record.IdempotencyKey || saved.Payload != record.Payload || saved.Source != record.Source {
			return ErrRuntimeQueueConflict
		}
		return nil
	})
	return &saved, err
}
