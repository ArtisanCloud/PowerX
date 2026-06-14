package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	env = strings.TrimSpace(env)
	if env == "" {
		return fmt.Errorf("env is required")
	}
	if agentID == 0 {
		return fmt.Errorf("agent_id is required")
	}
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
		}
	}()
	if err := tx.Error; err != nil {
		return err
	}
	normalized := normalizeSkillIDs(skillIDs)
	if len(normalized) == 0 {
		if err := tx.Model(&dbmodel.AgentSkillBinding{}).
			Scopes(dbmodel.WithScope(env, tenantUUID)).
			Where("agent_id = ?", agentID).
			Update("enabled", false).Error; err != nil {
			tx.Rollback()
			return err
		}
		return tx.Commit().Error
	}
	keep := make([]string, 0, len(normalized))
	now := time.Now()
	for i, id := range normalized {
		keep = append(keep, id)
		priority := (i + 1) * 10
		row := dbmodel.AgentSkillBinding{
			Env:        env,
			TenantUUID: tenantUUID,
			AgentID:    agentID,
			SkillID:    id,
			Priority:   priority,
			Enabled:    true,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "env"},
				{Name: "tenant_uuid"},
				{Name: "agent_id"},
				{Name: "skill_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"priority":   priority,
				"enabled":    true,
				"deleted_at": nil,
				"updated_at": now,
			}),
		}).Create(&row).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Model(&dbmodel.AgentSkillBinding{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("agent_id = ?", agentID).
		Where("skill_id NOT IN ?", keep).
		Update("enabled", false).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func normalizeSkillIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, id)
	}
	return out
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
