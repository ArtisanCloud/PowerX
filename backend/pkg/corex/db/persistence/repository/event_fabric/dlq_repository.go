package eventfabric

import (
	"context"
	"errors"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	baserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DlqRepository 管理死信消息的持久化、查询与状态变更。
type DlqRepository struct {
	base *baserepo.BaseRepository[eventfabricmodel.DlqMessage]
	db   *gorm.DB
}

func NewDlqRepository(db *gorm.DB) *DlqRepository {
	return &DlqRepository{
		base: baserepo.NewBaseRepository[eventfabricmodel.DlqMessage](db),
		db:   db,
	}
}

func (r *DlqRepository) WithDB(db *gorm.DB) *DlqRepository {
	if db == nil {
		return r
	}
	return NewDlqRepository(db)
}

func (r *DlqRepository) Create(ctx context.Context, message *eventfabricmodel.DlqMessage) (*eventfabricmodel.DlqMessage, error) {
	return r.base.Create(ctx, message)
}

func (r *DlqRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.DlqMessage, error) {
	var record eventfabricmodel.DlqMessage
	err := r.db.WithContext(ctx).Where("uuid = ?", id).Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *DlqRepository) List(ctx context.Context, tenantKey string, topic uuid.UUID, statuses []string, page, pageSize int) ([]*eventfabricmodel.DlqMessage, int64, error) {
	query := r.db.WithContext(ctx).Model(&eventfabricmodel.DlqMessage{}).Where("tenant_key = ?", tenantKey)
	if topic != uuid.Nil {
		query = query.Where("topic_uuid = ?", topic)
	}
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if pageSize > 0 {
		query = query.Limit(pageSize)
	}
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize)
	}

	var records []*eventfabricmodel.DlqMessage
	if err := query.Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func (r *DlqRepository) UpdateStatus(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&eventfabricmodel.DlqMessage{}).
		Where("uuid = ?", id).
		Updates(updates).Error
}

func (r *DlqRepository) PurgeByTopic(ctx context.Context, tenantKey string, topic uuid.UUID) (int64, error) {
	query := r.db.WithContext(ctx).Where("tenant_key = ?", tenantKey)
	if topic != uuid.Nil {
		query = query.Where("topic_uuid = ?", topic)
	}
	result := query.Delete(&eventfabricmodel.DlqMessage{})
	return result.RowsAffected, result.Error
}
