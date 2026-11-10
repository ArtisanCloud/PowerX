package eventfabric

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ServiceEventPrefix 将可靠服务投递与旧的内部总线自动确认路径隔离。
const ServiceEventPrefix = "efs1:"

var ErrServiceEventConflict = errors.New("event_fabric: idempotency conflict")

// ServiceDeliveryRepository 复用现有信封、投递及死信表，不建立第二套队列。
type ServiceDeliveryRepository struct{ db *gorm.DB }

func NewServiceDeliveryRepository(db *gorm.DB) *ServiceDeliveryRepository {
	return &ServiceDeliveryRepository{db: db}
}

func (r *ServiceDeliveryRepository) Topic(ctx context.Context, tenant, name string, id uuid.UUID) (*m.TopicDefinition, error) {
	var topic m.TopicDefinition
	query := r.db.WithContext(ctx).Where("tenant_key = ? AND status = ? AND lifecycle_status IN ?", tenant, 1, []string{"active", "deprecated"})
	if id != uuid.Nil {
		query = query.Where("uuid = ?", id)
	} else {
		query = query.Where("full_topic = ?", name)
	}
	if err := query.Take(&topic).Error; err != nil {
		return nil, err
	}
	return &topic, nil
}

// Publish 在同一事务中登记不可变信封及全部接收者。重复发布不覆盖原事件。
func (r *ServiceDeliveryRepository) Publish(ctx context.Context, envelope *m.EventEnvelope, recipients []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(envelope)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var old m.EventEnvelope
			if err := tx.Where("event_id = ? AND tenant_key = ?", envelope.EventID, envelope.TenantKey).Take(&old).Error; err != nil {
				return err
			}
			var oldHeaders, newHeaders any
			if json.Unmarshal(old.Headers, &oldHeaders) != nil || json.Unmarshal(envelope.Headers, &newHeaders) != nil || old.PayloadDigest != envelope.PayloadDigest || old.TopicUUID != envelope.TopicUUID || old.PublishedBy != envelope.PublishedBy || old.Version != envelope.Version || old.PayloadFormat != envelope.PayloadFormat || old.IdempotencyKey != envelope.IdempotencyKey || !reflect.DeepEqual(oldHeaders, newHeaders) {
				return ErrServiceEventConflict
			}
			*envelope = old
			return nil
		}
		for _, recipient := range recipients {
			row := &m.DeliveryAttempt{TenantKey: envelope.TenantKey, EnvelopeUUID: envelope.UUID, EventID: envelope.EventID, SubscriberID: recipient, DeliveryNo: 1, Status: "pending", TraceID: envelope.TraceID}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ServiceDeliveryRepository) Ready(ctx context.Context, tenant, subscriber string, topics []string, now time.Time, limit int) ([]m.DeliveryAttempt, error) {
	var rows []m.DeliveryAttempt
	topicIDs := r.db.Model(&m.TopicDefinition{}).Select("uuid").Where("tenant_key = ? AND full_topic IN ?", tenant, topics)
	envelopes := r.db.Model(&m.EventEnvelope{}).Select("uuid").Where("tenant_key = ? AND topic_uuid IN (?)", tenant, topicIDs)
	err := r.db.WithContext(ctx).Where("tenant_key = ? AND subscriber_id = ? AND event_id LIKE ?", tenant, subscriber, ServiceEventPrefix+"%").
		Where("envelope_uuid IN (?)", envelopes).
		Where("((status IN ? AND (scheduled_at IS NULL OR scheduled_at <= ?)) OR (status = ? AND scheduled_at <= ?))", []string{"pending", "scheduled"}, now, "delivering", now).
		Order("created_at ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

// WithDelivery 锁定认证租户及订阅者的单次投递；状态迁移、下一次尝试和死信原子提交。
// 不存在、跨租户及跨主体统一表现为未找到，避免泄露投递归属。
func (r *ServiceDeliveryRepository) WithDelivery(ctx context.Context, tenant, subscriber string, id uuid.UUID, apply func(*m.DeliveryAttempt, *m.EventEnvelope) (*m.DeliveryAttempt, *m.DlqMessage, error)) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row m.DeliveryAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND tenant_key = ? AND subscriber_id = ? AND event_id LIKE ?", id, tenant, subscriber, ServiceEventPrefix+"%").Take(&row).Error; err != nil {
			return err
		}
		var envelope m.EventEnvelope
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND tenant_key = ?", row.EnvelopeUUID, tenant).Take(&envelope).Error; err != nil {
			return err
		}
		next, dead, err := apply(&row, &envelope)
		if err != nil {
			return err
		}
		if err = tx.Save(&row).Error; err != nil {
			return err
		}
		if next != nil {
			if err = tx.Create(next).Error; err != nil {
				return err
			}
		}
		if dead != nil {
			if err = tx.Create(dead).Error; err != nil {
				return err
			}
		}
		var active, failed int64
		if err = tx.Model(&m.DeliveryAttempt{}).Where("envelope_uuid = ? AND status IN ?", envelope.UUID, []string{"pending", "scheduled", "delivering"}).Count(&active).Error; err != nil {
			return err
		}
		if active == 0 {
			if err = tx.Model(&m.DeliveryAttempt{}).Where("envelope_uuid = ? AND status = ? AND delivery_no >= ?", envelope.UUID, "failed", envelope.MaxRetry).Count(&failed).Error; err != nil {
				return err
			}
			envelope.Status = "succeeded"
			if failed > 0 {
				envelope.Status = "failed"
			}
		} else {
			envelope.Status = "pending"
		}
		return tx.Save(&envelope).Error
	})
}
