package repository

import (
	"context"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type AgentSkillBindingRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentSkillBinding]
	db *gorm.DB
}

func NewAgentSkillBindingRepository(db *gorm.DB) *AgentSkillBindingRepository {
	return &AgentSkillBindingRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentSkillBinding](db),
		db:             db,
	}
}

func (r *AgentSkillBindingRepository) Replace(ctx context.Context, env string, tenantUUID *string, agentID uint64, skillIDs []string) error {
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
		}
	}()
	if err := tx.Error; err != nil {
		return err
	}
	if err := tx.Where("agent_id = ?", agentID).Delete(&dbmodel.AgentSkillBinding{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if len(skillIDs) > 0 {
		rows := make([]dbmodel.AgentSkillBinding, 0, len(skillIDs))
		for i, id := range skillIDs {
			rows = append(rows, dbmodel.AgentSkillBinding{
				Env:        env,
				TenantUUID: tenantUUID,
				AgentID:    agentID,
				SkillID:    id,
				Priority:   (i + 1) * 10,
				Enabled:    true,
			})
		}
		if err := tx.Create(&rows).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

func (r *AgentSkillBindingRepository) ListByAgent(ctx context.Context, env string, tenantUUID *string, agentID uint64) ([]dbmodel.AgentSkillBinding, error) {
	var rows []dbmodel.AgentSkillBinding
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("agent_id = ?", agentID).
		Where("enabled = ?", true).
		Order("priority ASC, id ASC").
		Find(&rows).Error
	return rows, err
}
