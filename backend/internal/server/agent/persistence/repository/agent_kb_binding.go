package repository

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentKBBindingRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentKBBinding]
	db *gorm.DB
}

func NewAgentKBBindingRepository(db *gorm.DB) *AgentKBBindingRepository {
	return &AgentKBBindingRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentKBBinding](db),
		db:             db,
	}
}

// Attach / Upsert 唯一键：agent_id + kb_id
func (r *AgentKBBindingRepository) Attach(ctx context.Context, env string, tenantUUID *string, in *dbmodel.AgentKBBinding) error {
	tx := r.db.WithContext(ctx)
	in.Env = env
	in.TenantUUID = tenantUUID

	assign := clause.Assignments(map[string]any{
		"weight":       in.Weight,
		"status":       in.Status,
		"index_policy": in.IndexPolicy,
		"updated_at":   gorm.Expr("NOW()"),
	})
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}, {Name: "kb_id"}},
		DoUpdates: assign,
	}).Create(in).Error
}

func (r *AgentKBBindingRepository) Detach(ctx context.Context, agentID, kbID uint64) error {
	return r.db.WithContext(ctx).
		Where("agent_id = ? AND kb_id = ?", agentID, kbID).
		Delete(&dbmodel.AgentKBBinding{}).Error
}

func (r *AgentKBBindingRepository) ListByAgent(ctx context.Context, env string, tenantUUID *string, agentID uint64) ([]dbmodel.AgentKBBinding, error) {
	var list []dbmodel.AgentKBBinding
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("agent_id = ?", agentID).
		Order("kb_id ASC").
		Find(&list).Error
	return list, err
}

// Replace：用新集合替换 agent 的全部绑定（在事务外部控制 tx 更佳）
func (r *AgentKBBindingRepository) Replace(ctx context.Context, env string, tenantUUID *string, agentID uint64, bindings []dbmodel.AgentKBBinding) error {
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	if err := tx.Error; err != nil {
		return err
	}
	// 统一作用域
	for i := range bindings {
		bindings[i].Env = env
		bindings[i].TenantUUID = tenantUUID
		bindings[i].AgentID = agentID
	}

	// 先删旧
	if err := tx.Where("agent_id = ?", agentID).
		Delete(&dbmodel.AgentKBBinding{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	// 再批量插入
	if len(bindings) > 0 {
		if err := tx.Create(&bindings).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}
