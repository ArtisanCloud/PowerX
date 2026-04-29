package repository

import (
	"context"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type AgentKnowledgeBindingRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentKnowledgeBinding]
	db *gorm.DB
}

func NewAgentKnowledgeBindingRepository(db *gorm.DB) *AgentKnowledgeBindingRepository {
	return &AgentKnowledgeBindingRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentKnowledgeBinding](db),
		db:             db,
	}
}

func (r *AgentKnowledgeBindingRepository) Replace(ctx context.Context, env string, tenantUUID *string, agentID uint64, knowledgeSpaceUUIDs []string) error {
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
		}
	}()
	if err := tx.Error; err != nil {
		return err
	}
	if err := tx.Where("agent_id = ?", agentID).Delete(&dbmodel.AgentKnowledgeBinding{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if len(knowledgeSpaceUUIDs) > 0 {
		rows := make([]dbmodel.AgentKnowledgeBinding, 0, len(knowledgeSpaceUUIDs))
		for _, id := range knowledgeSpaceUUIDs {
			rows = append(rows, dbmodel.AgentKnowledgeBinding{
				Env:                env,
				TenantUUID:         tenantUUID,
				AgentID:            agentID,
				KnowledgeSpaceUUID: id,
				Weight:             1,
				Enabled:            true,
			})
		}
		if err := tx.Create(&rows).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

func (r *AgentKnowledgeBindingRepository) ListByAgent(ctx context.Context, env string, tenantUUID *string, agentID uint64) ([]dbmodel.AgentKnowledgeBinding, error) {
	var rows []dbmodel.AgentKnowledgeBinding
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("agent_id = ?", agentID).
		Where("enabled = ?", true).
		Order("id ASC").
		Find(&rows).Error
	return rows, err
}
