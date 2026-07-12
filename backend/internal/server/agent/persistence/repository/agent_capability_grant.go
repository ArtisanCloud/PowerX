package repository

import (
	"context"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AgentCapabilityGrantRepository persists Agent capability authorization grants.
type AgentCapabilityGrantRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentCapabilityGrant]
	db *gorm.DB
}

func NewAgentCapabilityGrantRepository(db *gorm.DB) *AgentCapabilityGrantRepository {
	return &AgentCapabilityGrantRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentCapabilityGrant](db),
		db:             db,
	}
}

func (r *AgentCapabilityGrantRepository) ListByAgent(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID) ([]dbmodel.AgentCapabilityGrant, error) {
	var rows []dbmodel.AgentCapabilityGrant
	err := r.db.WithContext(ctx).
		Where("env = ? AND tenant_uuid = ? AND agent_uuid = ?", strings.TrimSpace(env), strings.TrimSpace(tenantUUID), agentUUID).
		Order("plugin_id ASC, capability_id ASC, permission_code ASC").
		Find(&rows).Error
	return rows, err
}

func (r *AgentCapabilityGrantRepository) ReplaceByAgent(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, rows []dbmodel.AgentCapabilityGrant) error {
	env = strings.TrimSpace(env)
	tenantUUID = strings.TrimSpace(tenantUUID)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("env = ? AND tenant_uuid = ? AND agent_uuid = ?", env, tenantUUID, agentUUID).
			Delete(&dbmodel.AgentCapabilityGrant{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for i := range rows {
			rows[i].Env = env
			rows[i].TenantUUID = tenantUUID
			rows[i].AgentUUID = agentUUID
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "env"},
				{Name: "tenant_uuid"},
				{Name: "agent_uuid"},
				{Name: "capability_uuid"},
				{Name: "permission_code"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"capability_id",
				"plugin_id",
				"plugin_uuid",
				"risk_level",
				"status",
				"source",
				"updated_by_user_uuid",
				"updated_at",
			}),
		}).Create(&rows).Error
	})
}

func (r *AgentCapabilityGrantRepository) UpsertByAgent(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, rows []dbmodel.AgentCapabilityGrant) error {
	env = strings.TrimSpace(env)
	tenantUUID = strings.TrimSpace(tenantUUID)
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].Env = env
		rows[i].TenantUUID = tenantUUID
		rows[i].AgentUUID = agentUUID
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "env"},
			{Name: "tenant_uuid"},
			{Name: "agent_uuid"},
			{Name: "capability_uuid"},
			{Name: "permission_code"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"capability_id",
			"plugin_id",
			"plugin_uuid",
			"risk_level",
			"status",
			"source",
			"updated_by_user_uuid",
			"updated_at",
		}),
	}).Create(&rows).Error
}

func (r *AgentCapabilityGrantRepository) ReplaceByAgentSource(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, source string, rows []dbmodel.AgentCapabilityGrant) error {
	env = strings.TrimSpace(env)
	tenantUUID = strings.TrimSpace(tenantUUID)
	source = strings.TrimSpace(source)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("env = ? AND tenant_uuid = ? AND agent_uuid = ? AND source = ?", env, tenantUUID, agentUUID, source).
			Delete(&dbmodel.AgentCapabilityGrant{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for i := range rows {
			rows[i].Env = env
			rows[i].TenantUUID = tenantUUID
			rows[i].AgentUUID = agentUUID
			rows[i].Source = source
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "env"},
				{Name: "tenant_uuid"},
				{Name: "agent_uuid"},
				{Name: "capability_uuid"},
				{Name: "permission_code"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"capability_id",
				"plugin_id",
				"plugin_uuid",
				"risk_level",
				"status",
				"source",
				"updated_by_user_uuid",
				"updated_at",
			}),
		}).Create(&rows).Error
	})
}

func (r *AgentCapabilityGrantRepository) HasEnabledGrant(ctx context.Context, env, tenantUUID string, agentUUID uuid.UUID, capabilityID, permissionCode string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&dbmodel.AgentCapabilityGrant{}).
		Where("env = ? AND tenant_uuid = ? AND agent_uuid = ?", strings.TrimSpace(env), strings.TrimSpace(tenantUUID), agentUUID).
		Where("capability_id = ? AND permission_code = ? AND status = ?", strings.TrimSpace(capabilityID), strings.TrimSpace(permissionCode), dbmodel.AgentCapabilityGrantStatusEnabled).
		Count(&count).Error
	return count > 0, err
}
