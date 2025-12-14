package repository

import (
	"context"
	"fmt"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentSettingRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentSetting]
	db *gorm.DB
}

func NewAgentSettingRepository(db *gorm.DB) *AgentSettingRepository {
	return &AgentSettingRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentSetting](db),
		db:             db,
	}
}

// Upsert 唯一键：
// - 全局：env + agent_id（且 tenant_uuid IS NULL）
// - 租户：env + tenant_uuid + agent_id
func (r *AgentSettingRepository) UpsertByAgent(
	ctx context.Context, env string, tenantUUID *string, in *dbmodel.AgentSetting,
) error {
	if tenantUUID == nil {
		return fmt.Errorf("UpsertByAgent: tenantUUID must not be nil (system tenant=uuid)")
	}
	tx := r.db.WithContext(ctx)
	in.Env = env
	in.TenantUUID = tenantUUID

	assign := clause.Assignments(map[string]any{
		"provider":       in.Provider,
		"model":          in.Model,
		"params":         in.Params,
		"override_flags": in.OverrideFlags,
		"quota_policy":   in.QuotaPolicy,
		"health_status":  in.HealthStatus,
		"health_info":    in.HealthInfo,
		"updated_at":     gorm.Expr("NOW()"),
	})

	return tx.
		Clauses(
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "env"}, {Name: "tenant_uuid"}, {Name: "agent_id"}},
				DoUpdates: assign,
			},
			clause.Returning{Columns: []clause.Column{{Name: "id"}}},
		).
		Create(in).Error
}

func (r *AgentSettingRepository) FindByAgent(ctx context.Context, env string, tenantUUID *string, agentID uint64) (*dbmodel.AgentSetting, error) {
	var out dbmodel.AgentSetting
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("agent_id = ?", agentID).
		First(&out).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AgentSettingRepository) UpdateHealth(ctx context.Context, id uint64, status string, info datatypes.JSONMap) error {
	return r.db.WithContext(ctx).Model(&dbmodel.AgentSetting{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"health_status": status,
			"health_info":   info,
		}).Error
}
