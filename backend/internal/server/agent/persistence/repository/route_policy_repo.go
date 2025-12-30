package repository

import (
	"context"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AIRoutePolicyRepository struct {
	*coreRepo.BaseRepository[dbmodel.AIRoutePolicy]
	db *gorm.DB
}

func NewAIRoutePolicyRepository(db *gorm.DB) *AIRoutePolicyRepository {
	return &AIRoutePolicyRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AIRoutePolicy](db),
		db:             db,
	}
}

func (r *AIRoutePolicyRepository) UpsertDefaultByScopeModality(
	ctx context.Context, env string, tenantUUID *string,
	modality, provider, model string,
) error {
	rec := &dbmodel.AIRoutePolicy{
		Env:      env,
		TenantUUID: tenantUUID,
		Modality: modality,
		Provider: provider,
		Model:    model,
		// 其余字段（strategy/compliance/quota）按需赋
	}
	return r.UpsertByScopeSelectors(ctx, rec)
}

// Upsert 唯一键：env + tenant_uuid + modality + agent_id(NULL 可选) + flow_id(NULL 可选) + purpose(NULL 可选)
func (r *AIRoutePolicyRepository) UpsertByScopeSelectors(ctx context.Context, in *dbmodel.AIRoutePolicy) error {
	tx := r.db.WithContext(ctx)

	var old dbmodel.AIRoutePolicy
	q := tx.Scopes(dbmodel.WithScope(in.Env, in.TenantUUID)).
		Where("modality = ?", in.Modality)

	if in.AgentID != nil {
		q = q.Where("agent_id = ?", *in.AgentID)
	} else {
		q = q.Where("agent_id IS NULL")
	}
	if in.FlowID != nil {
		q = q.Where("flow_id = ?", *in.FlowID)
	} else {
		q = q.Where("flow_id IS NULL")
	}
	if in.Purpose != nil {
		q = q.Where("purpose = ?", *in.Purpose)
	} else {
		q = q.Where("purpose IS NULL")
	}

	err := q.First(&old).Error
	switch err {
	case nil:
		in.ID = old.ID
		return tx.Save(in).Error
	case gorm.ErrRecordNotFound:
		return tx.Create(in).Error
	default:
		return err
	}
}

func (r *AIRoutePolicyRepository) FindDefaultByScopeModality(
	ctx context.Context, env string, tenantUUID *string,
	modality string,
) (*dbmodel.AIRoutePolicy, error) {
	return r.FindByScopeSelectors(ctx, env, tenantUUID, modality, nil, nil, nil)
}

func (r *AIRoutePolicyRepository) FindByScopeSelectors(
	ctx context.Context,
	env string, tenantUUID *string,
	modality string,
	agentID, flowID, purpose *string,
) (*dbmodel.AIRoutePolicy, error) {

	tx := r.db.WithContext(ctx)
	var out dbmodel.AIRoutePolicy

	q := tx.Scopes(dbmodel.WithScope(env, tenantUUID)).Where("modality = ?", modality)
	if agentID != nil {
		q = q.Where("agent_id = ?", *agentID)
	} else {
		q = q.Where("agent_id IS NULL")
	}
	if flowID != nil {
		q = q.Where("flow_id = ?", *flowID)
	} else {
		q = q.Where("flow_id IS NULL")
	}
	if purpose != nil {
		q = q.Where("purpose = ?", *purpose)
	} else {
		q = q.Where("purpose IS NULL")
	}

	if err := q.First(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AIRoutePolicyRepository) ListByModality(ctx context.Context, env string, tenantUUID *string, modality string) ([]dbmodel.AIRoutePolicy, error) {
	var list []dbmodel.AIRoutePolicy
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("modality = ?", modality).
		Order("agent_id NULLS FIRST, flow_id NULLS FIRST, purpose NULLS FIRST, id ASC").
		Find(&list).Error
	return list, err
}

func (r *AIRoutePolicyRepository) DeleteByID(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&dbmodel.AIRoutePolicy{}, id).Error
}

// Resolve：按优先级挑选策略（精确>flow+purpose>agent+purpose>flow>agent>purpose>全 NULL）
func (r *AIRoutePolicyRepository) Resolve(
	ctx context.Context,
	env string, tenantUUID *string,
	modality string,
	agentID, flowID, purpose *string,
) (*dbmodel.AIRoutePolicy, error) {

	tx := r.db.WithContext(ctx)
	var out dbmodel.AIRoutePolicy
	base := tx.Scopes(dbmodel.WithScope(env, tenantUUID)).Model(&dbmodel.AIRoutePolicy{}).Where("modality = ?", modality)

	// 1) agent+flow+purpose
	if agentID != nil && flowID != nil && purpose != nil {
		if err := base.Where("agent_id = ? AND flow_id = ? AND purpose = ?", *agentID, *flowID, *purpose).
			First(&out).Error; err == nil {
			return &out, nil
		}
	}
	// 2) flow+purpose
	if flowID != nil && purpose != nil {
		if err := base.Where("flow_id = ? AND purpose = ?", *flowID, *purpose).
			First(&out).Error; err == nil {
			return &out, nil
		}
	}
	// 3) agent+purpose
	if agentID != nil && purpose != nil {
		if err := base.Where("agent_id = ? AND purpose = ?", *agentID, *purpose).
			First(&out).Error; err == nil {
			return &out, nil
		}
	}
	// 4) flow
	if flowID != nil {
		if err := base.Where("flow_id = ?", *flowID).First(&out).Error; err == nil {
			return &out, nil
		}
	}
	// 5) agent
	if agentID != nil {
		if err := base.Where("agent_id = ?", *agentID).First(&out).Error; err == nil {
			return &out, nil
		}
	}
	// 6) purpose
	if purpose != nil {
		if err := base.Where("purpose = ?", *purpose).First(&out).Error; err == nil {
			return &out, nil
		}
	}
	// 7) 全 NULL（通配）
	if err := base.
		Where("agent_id IS NULL AND flow_id IS NULL AND purpose IS NULL").
		First(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// 可选：快速设置策略体（便捷更新）
func (r *AIRoutePolicyRepository) UpdateBodies(
	ctx context.Context, id uint64,
	strategy, compliance, quota datatypes.JSONMap,
) error {
	return r.db.WithContext(ctx).Model(&dbmodel.AIRoutePolicy{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"strategy":   strategy,
			"compliance": compliance,
			"quota":      quota,
		}).Error
}
