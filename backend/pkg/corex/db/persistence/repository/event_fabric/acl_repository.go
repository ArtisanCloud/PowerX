package eventfabric

import (
	"context"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	baserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AclRepository struct {
	base *baserepo.BaseRepository[eventfabricmodel.AclBinding]
	db   *gorm.DB
}

func NewAclRepository(db *gorm.DB) *AclRepository {
	return &AclRepository{
		base: baserepo.NewBaseRepository[eventfabricmodel.AclBinding](db),
		db:   db,
	}
}

func (r *AclRepository) WithDB(db *gorm.DB) *AclRepository {
	if db == nil {
		return r
	}
	return &AclRepository{
		base: baserepo.NewBaseRepository[eventfabricmodel.AclBinding](db),
		db:   db,
	}
}

func (r *AclRepository) UpsertBindings(ctx context.Context, bindings []*eventfabricmodel.AclBinding) ([]*eventfabricmodel.AclBinding, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	query := r.db.WithContext(ctx)
	err := query.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_key"}, {Name: "topic_uuid"}, {Name: "principal_id"}, {Name: "action"}},
		DoUpdates: clause.AssignmentColumns([]string{"expires_at", "granted_by", "justification", "audit_ref", "status", "updated_at"}),
	}).Create(&bindings).Error
	if err != nil {
		return nil, err
	}
	return bindings, nil
}

func (r *AclRepository) RemoveBindings(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, actions []string) (int64, error) {
	query := r.db.WithContext(ctx).Where("tenant_key = ? AND topic_uuid = ? AND principal_id = ?", tenantKey, topic, principalID)
	if len(actions) > 0 {
		query = query.Where("action IN ?", actions)
	}
	result := query.Delete(&eventfabricmodel.AclBinding{})
	return result.RowsAffected, result.Error
}

func (r *AclRepository) ListByTopic(ctx context.Context, tenantKey string, topic uuid.UUID) ([]*eventfabricmodel.AclBinding, error) {
	var rows []*eventfabricmodel.AclBinding
	err := r.db.WithContext(ctx).
		Where("tenant_key = ? AND topic_uuid = ?", tenantKey, topic).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *AclRepository) HasPermission(ctx context.Context, tenantKey string, topic uuid.UUID, principalID string, action string, now time.Time) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&eventfabricmodel.AclBinding{}).
		Where("tenant_key = ? AND topic_uuid = ? AND principal_id = ? AND action = ?", tenantKey, topic, principalID, action).
		Where("status = ?", 1).
		Where("expires_at IS NULL OR expires_at > ?", now).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
