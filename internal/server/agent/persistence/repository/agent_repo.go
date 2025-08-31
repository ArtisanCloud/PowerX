// internal/server/agent/persistence/repository/agent_repo.go
package repository

import (
	"context"
	"fmt"
	"gorm.io/gorm/clause"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

// ======================= AgentRepository =======================

type AgentRepository struct {
	*coreRepo.BaseRepository[dbmodel.Agent]
	db *gorm.DB
}

func NewAgentRepository(db *gorm.DB) *AgentRepository {
	return &AgentRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.Agent](db),
		db:             db,
	}
}

// Upsert 唯一键：
// - 全局：env + key（且 tenant_id IS NULL）
// - 租户：env + tenant_id + key
// 当 tenantID == nil（system 级）时，必须在 ON CONFLICT 目标上加 WHERE tenant_id IS NULL
// ---------- AgentRepository.UpsertByScopeKey（精简版：仅租户级） ----------
func (r *AgentRepository) UpsertByScopeKey(
	ctx context.Context, env string, tenantID *uint64, in *dbmodel.Agent,
) error {
	if tenantID == nil {
		return fmt.Errorf("UpsertByScopeKey: tenantID must not be nil (system tenant=1)")
	}
	tx := r.db.WithContext(ctx)
	in.Env = env
	in.TenantID = tenantID

	assign := clause.Assignments(map[string]any{
		"name":               in.Name,
		"description":        in.Description,
		"source":             in.Source,
		"scope":              in.Scope,
		"visibility":         in.Visibility,
		"status":             in.Status,
		"default_persona_id": in.DefaultPersonaID,
		"blueprint_refs":     in.BlueprintRefs,
		"intent_cards_ref":   in.IntentCardsRef,
		"tool_allowlist":     in.ToolAllowlist,
		"kb_strategy":        in.KBStrategy,
		"meta":               in.Meta,
		"updated_at":         gorm.Expr("NOW()"),
	})

	return tx.
		Clauses(
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "env"}, {Name: "tenant_id"}, {Name: "key"}},
				DoUpdates: assign,
			},
			clause.Returning{Columns: []clause.Column{{Name: "id"}}},
		).
		Create(in).Error
}

func (r *AgentRepository) FindByScopeKey(ctx context.Context, env string, tenantID *uint64, key string) (*dbmodel.Agent, error) {
	var out dbmodel.Agent
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Where(`key = ?`, key).
		First(&out).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AgentRepository) GetByID(ctx context.Context, id uint64) (*dbmodel.Agent, error) {
	var out dbmodel.Agent
	if err := r.db.WithContext(ctx).First(&out, id).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AgentRepository) ListByScope(ctx context.Context, env string, tenantID *uint64, statuses ...string) ([]dbmodel.Agent, error) {
	tx := r.db.WithContext(ctx).Scopes(dbmodel.WithScope(env, tenantID)).Model(&dbmodel.Agent{})
	if len(statuses) > 0 {
		tx = tx.Where("status IN ?", statuses)
	}
	var list []dbmodel.Agent
	if err := tx.Order("status, key").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *AgentRepository) UpdateStatus(ctx context.Context, id uint64, status string) error {
	return r.db.WithContext(ctx).
		Model(&dbmodel.Agent{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (r *AgentRepository) Enable(ctx context.Context, id uint64) error {
	return r.UpdateStatus(ctx, id, dbmodel.AgentStatusActive)
}
func (r *AgentRepository) Disable(ctx context.Context, id uint64) error {
	return r.UpdateStatus(ctx, id, dbmodel.AgentStatusDisabled)
}
func (r *AgentRepository) MarkBroken(ctx context.Context, id uint64) error {
	return r.UpdateStatus(ctx, id, dbmodel.AgentStatusBroken)
}

func (r *AgentRepository) DeleteSoft(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&dbmodel.Agent{}, id).Error
}

// 批量按插件来源联动状态（插件禁用/卸载时调用）
func (r *AgentRepository) UpdateStatusByPlugin(ctx context.Context, env string, tenantID *uint64, pluginID, status string) error {
	return r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Model(&dbmodel.Agent{}).
		Where("source = ?", "plugin:"+pluginID).
		Update("status", status).Error
}
