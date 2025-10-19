package eventfabric

import (
	"context"
	"errors"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	baserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DeliveryRepository 封装重试与投递记录的数据库操作。
type DeliveryRepository struct {
	base *baserepo.BaseRepository[eventfabricmodel.DeliveryAttempt]
	db   *gorm.DB
}

func NewDeliveryRepository(db *gorm.DB) *DeliveryRepository {
	return &DeliveryRepository{
		base: baserepo.NewBaseRepository[eventfabricmodel.DeliveryAttempt](db),
		db:   db,
	}
}

func (r *DeliveryRepository) WithDB(db *gorm.DB) *DeliveryRepository {
	if db == nil {
		return r
	}
	return NewDeliveryRepository(db)
}

func (r *DeliveryRepository) CreateAttempt(ctx context.Context, attempt *eventfabricmodel.DeliveryAttempt) (*eventfabricmodel.DeliveryAttempt, error) {
	return r.base.Create(ctx, attempt)
}

// UpsertAttempt 以 tenant/event/subscriber 为幂等键创建或更新投递记录。
func (r *DeliveryRepository) UpsertAttempt(ctx context.Context, attempt *eventfabricmodel.DeliveryAttempt) (*eventfabricmodel.DeliveryAttempt, error) {
	query := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_key"}, {Name: "event_id"}, {Name: "subscriber_id"}, {Name: "delivery_no"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "latency_ms", "last_error_code", "nack_reason", "scheduled_at", "last_attempt_at", "acked_at"}),
	}).Create(attempt)
	if query.Error != nil {
		return nil, query.Error
	}
	if query.RowsAffected == 0 {
		return r.FindByEnvelopeAndSubscriber(ctx, attempt.EnvelopeUUID, attempt.SubscriberID)
	}
	return attempt, nil
}

func (r *DeliveryRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.DeliveryAttempt, error) {
	var record eventfabricmodel.DeliveryAttempt
	err := r.db.WithContext(ctx).
		Where("uuid = ?", id).
		Take(&record).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *DeliveryRepository) FindByEnvelopeAndSubscriber(ctx context.Context, envelope uuid.UUID, subscriberID string) (*eventfabricmodel.DeliveryAttempt, error) {
	var record eventfabricmodel.DeliveryAttempt
	err := r.db.WithContext(ctx).
		Where("envelope_uuid = ? AND subscriber_id = ?", envelope, subscriberID).
		Order("delivery_no DESC").
		Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *DeliveryRepository) UpdateStatus(ctx context.Context, attemptUUID uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&eventfabricmodel.DeliveryAttempt{}).
		Where("uuid = ?", attemptUUID).
		Updates(updates).Error
}

// ListDueRetries 返回到达重试时间的投递记录。
func (r *DeliveryRepository) ListDueRetries(ctx context.Context, tenantKey string, before time.Time, limit int) ([]*eventfabricmodel.DeliveryAttempt, error) {
	query := r.db.WithContext(ctx).
		Where("tenant_key = ? AND status = ? AND scheduled_at IS NOT NULL AND scheduled_at <= ?", tenantKey, "scheduled", before).
		Order("scheduled_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var attempts []*eventfabricmodel.DeliveryAttempt
	if err := query.Find(&attempts).Error; err != nil {
		return nil, err
	}
	return attempts, nil
}

func (r *DeliveryRepository) CountActiveAttempts(ctx context.Context, envelope uuid.UUID) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).
		Model(&eventfabricmodel.DeliveryAttempt{}).
		Where("envelope_uuid = ? AND status IN ?", envelope, []string{"pending", "delivering"}).
		Count(&total).Error
	return total, err
}
