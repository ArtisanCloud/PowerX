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
	"gorm.io/gorm/clause"
)

// EnvelopeRepository 负责事件信封的持久化与查询。
type EnvelopeRepository struct {
	base *baserepo.BaseRepository[eventfabricmodel.EventEnvelope]
	db   *gorm.DB
}

// ReplayQuery 描述回放检索条件。
type ReplayQuery struct {
	Statuses  []string
	Limit     int
	StartTime time.Time
	EndTime   time.Time
	TraceID   string
}

func NewEnvelopeRepository(db *gorm.DB) *EnvelopeRepository {
	return &EnvelopeRepository{
		base: baserepo.NewBaseRepository[eventfabricmodel.EventEnvelope](db),
		db:   db,
	}
}

func (r *EnvelopeRepository) WithDB(db *gorm.DB) *EnvelopeRepository {
	if db == nil {
		return r
	}
	return NewEnvelopeRepository(db)
}

// Create 插入新的事件信封。
func (r *EnvelopeRepository) Create(ctx context.Context, envelope *eventfabricmodel.EventEnvelope) (*eventfabricmodel.EventEnvelope, error) {
	return r.base.Create(ctx, envelope)
}

// UpsertByEventID 基于租户与 event_id 进行幂等插入，若已存在则返回现有记录。
func (r *EnvelopeRepository) UpsertByEventID(ctx context.Context, envelope *eventfabricmodel.EventEnvelope) (*eventfabricmodel.EventEnvelope, bool, error) {
	query := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_key"}, {Name: "event_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "headers", "payload_format", "payload_digest", "published_by", "trace_id", "status", "max_retry", "ack_timeout_sec"}),
	}).Create(envelope)
	if query.Error != nil {
		return nil, false, query.Error
	}

	if query.RowsAffected == 0 {
		existing, err := r.FindByEventID(ctx, envelope.TenantKey, envelope.EventID)
		return existing, true, err
	}
	return envelope, false, nil
}

func (r *EnvelopeRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.EventEnvelope, error) {
	var record eventfabricmodel.EventEnvelope
	err := r.db.WithContext(ctx).Where("uuid = ?", id).Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *EnvelopeRepository) FindByEventID(ctx context.Context, tenantKey, eventID string) (*eventfabricmodel.EventEnvelope, error) {
	var record eventfabricmodel.EventEnvelope
	err := r.db.WithContext(ctx).
		Where("tenant_key = ? AND event_id = ?", tenantKey, eventID).
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

// UpdateStatus 更新事件的状态与重试信息。
func (r *EnvelopeRepository) UpdateStatus(ctx context.Context, envelopeUUID uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&eventfabricmodel.EventEnvelope{}).
		Where("uuid = ?", envelopeUUID).
		Updates(updates).Error
}

// ListForReplay 返回指定条件的事件信封，供回放使用。
func (r *EnvelopeRepository) ListForReplay(ctx context.Context, tenantKey string, topic uuid.UUID, filter ReplayQuery) ([]*eventfabricmodel.EventEnvelope, error) {
	query := r.db.WithContext(ctx).Model(&eventfabricmodel.EventEnvelope{}).Where("tenant_key = ?", tenantKey)
	if topic != uuid.Nil {
		query = query.Where("topic_uuid = ?", topic)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	if !filter.StartTime.IsZero() {
		query = query.Where("published_at >= ?", filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query = query.Where("published_at <= ?", filter.EndTime)
	}
	if strings.TrimSpace(filter.TraceID) != "" {
		query = query.Where("trace_id = ?", strings.TrimSpace(filter.TraceID))
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	var records []*eventfabricmodel.EventEnvelope
	if err := query.Order("published_at ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
