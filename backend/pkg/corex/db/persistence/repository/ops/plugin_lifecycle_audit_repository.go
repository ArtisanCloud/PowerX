package ops

import (
	"context"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type PluginLifecycleAuditRepository struct {
	*repository.BaseRepository[modelops.PluginLifecycleAudit]
	db *gorm.DB
}

func NewPluginLifecycleAuditRepository(db *gorm.DB) *PluginLifecycleAuditRepository {
	return &PluginLifecycleAuditRepository{
		BaseRepository: repository.NewBaseRepository[modelops.PluginLifecycleAudit](db),
		db:             db,
	}
}

func (r *PluginLifecycleAuditRepository) ListByPluginID(ctx context.Context, pluginID string, limit, offset int) ([]modelops.PluginLifecycleAudit, int64, error) {
	q := r.db.WithContext(ctx).Model(&modelops.PluginLifecycleAudit{}).Where("plugin_id = ?", pluginID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []modelops.PluginLifecycleAudit
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
