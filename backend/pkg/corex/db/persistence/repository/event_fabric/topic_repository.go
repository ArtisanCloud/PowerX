package eventfabric

import (
	"context"
	"errors"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	baserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TopicRepository 提供事件主题的仓储能力。
type TopicRepository struct {
	base *baserepo.BaseRepository[eventfabricmodel.TopicDefinition]
	db   *gorm.DB
}

func NewTopicRepository(db *gorm.DB) *TopicRepository {
	return &TopicRepository{
		base: baserepo.NewBaseRepository[eventfabricmodel.TopicDefinition](db),
		db:   db,
	}
}

func (r *TopicRepository) WithDB(db *gorm.DB) *TopicRepository {
	if db == nil {
		return r
	}
	return &TopicRepository{
		base: baserepo.NewBaseRepository[eventfabricmodel.TopicDefinition](db),
		db:   db,
	}
}

func (r *TopicRepository) Create(ctx context.Context, topic *eventfabricmodel.TopicDefinition) (*eventfabricmodel.TopicDefinition, error) {
	return r.base.Create(ctx, topic)
}

func (r *TopicRepository) Update(ctx context.Context, topic *eventfabricmodel.TopicDefinition) (*eventfabricmodel.TopicDefinition, error) {
	query := r.db.WithContext(ctx)
	if err := query.Save(topic).Error; err != nil {
		return nil, err
	}
	return topic, nil
}

func (r *TopicRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.TopicDefinition, error) {
	var record eventfabricmodel.TopicDefinition
	if err := r.db.WithContext(ctx).Where("uuid = ?", id).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *TopicRepository) FindByComposite(ctx context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error) {
	var record eventfabricmodel.TopicDefinition
	err := r.db.WithContext(ctx).
		Where("tenant_key = ? AND namespace = ? AND name = ?", tenantKey, namespace, name).
		Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *TopicRepository) List(ctx context.Context, queryCtx QueryContext) ([]*eventfabricmodel.TopicDefinition, int64, error) {
	query := r.db.WithContext(ctx).Model(&eventfabricmodel.TopicDefinition{})

	if queryCtx.Filter.TenantID != "" {
		query = query.Where("tenant_key = ?", queryCtx.Filter.TenantID)
	}
	if queryCtx.Filter.Namespace != "" {
		query = query.Where("namespace = ?", queryCtx.Filter.Namespace)
	}
	if len(queryCtx.Filter.Lifecycle) > 0 {
		query = query.Where("lifecycle_status IN ?", queryCtx.Filter.Lifecycle)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if queryCtx.Page.Limit > 0 {
		query = query.Limit(queryCtx.Page.Limit)
	}
	if queryCtx.Page.Offset > 0 {
		query = query.Offset(queryCtx.Page.Offset)
	}

	if queryCtx.Sort.Field != "" {
		order := queryCtx.Sort.Field
		if queryCtx.Sort.Desc {
			order += " DESC"
		}
		query = query.Order(order)
	} else {
		query = query.Order("created_at DESC")
	}

	var records []*eventfabricmodel.TopicDefinition
	if err := query.Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}
