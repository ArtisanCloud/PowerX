package repository

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentPluginLinkRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentPluginLink]
	db *gorm.DB
}

func NewAgentPluginLinkRepository(db *gorm.DB) *AgentPluginLinkRepository {
	return &AgentPluginLinkRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentPluginLink](db),
		db:             db,
	}
}

// Upsert 唯一键：env + tenant_id + plugin_id + plugin_agent_key
func (r *AgentPluginLinkRepository) UpsertByScopePluginKey(ctx context.Context, env string, tenantID *uint64, in *dbmodel.AgentPluginLink) error {
	tx := r.db.WithContext(ctx)
	in.Env = env
	in.TenantID = tenantID

	assign := clause.Assignments(map[string]any{
		"agent_id":       in.AgentID,
		"install_status": in.InstallStatus,
		"updated_at":     gorm.Expr("NOW()"),
	})

	var conflict clause.OnConflict
	if tenantID != nil {
		conflict = clause.OnConflict{
			Columns: []clause.Column{
				{Name: "env"}, {Name: "tenant_id"},
				{Name: "plugin_id"}, {Name: "plugin_agent_key"},
			},
			DoUpdates: assign,
		}
	} else {
		conflict = clause.OnConflict{
			Columns: []clause.Column{
				{Name: "env"},
				{Name: "plugin_id"}, {Name: "plugin_agent_key"},
			},
			DoUpdates: assign,
		}
	}
	return tx.Clauses(conflict).Create(in).Error
}

func (r *AgentPluginLinkRepository) ListByPlugin(ctx context.Context, env string, tenantID *uint64, pluginID string) ([]dbmodel.AgentPluginLink, error) {
	var list []dbmodel.AgentPluginLink
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Where("plugin_id = ?", pluginID).
		Order("plugin_agent_key ASC").
		Find(&list).Error
	return list, err
}

func (r *AgentPluginLinkRepository) UpdateInstallStatus(ctx context.Context, id uint64, status string) error {
	return r.db.WithContext(ctx).Model(&dbmodel.AgentPluginLink{}).
		Where("id = ?", id).
		Update("install_status", status).Error
}

func (r *AgentPluginLinkRepository) DeleteByID(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&dbmodel.AgentPluginLink{}, id).Error
}
