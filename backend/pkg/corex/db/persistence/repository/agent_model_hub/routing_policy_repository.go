package agent_model_hub

import (
	"context"
	"database/sql"
	"errors"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

// RoutingPolicyRepository encapsulates persistence for routing governance records.
type RoutingPolicyRepository struct {
	*base.BaseRepository[model.RoutingPolicy]
	db *gorm.DB
}

func NewRoutingPolicyRepository(db *gorm.DB) *RoutingPolicyRepository {
	return &RoutingPolicyRepository{
		BaseRepository: base.NewBaseRepository[model.RoutingPolicy](db),
		db:             db,
	}
}

func (r *RoutingPolicyRepository) WithDB(db *gorm.DB) *RoutingPolicyRepository {
	return NewRoutingPolicyRepository(db)
}

// NextVersion calculates the next monotonic version per (env, tenant_scope).
func (r *RoutingPolicyRepository) NextVersion(ctx context.Context, env, tenantScope string) (uint32, error) {
	var maxVersion sql.NullInt64
	err := r.db.WithContext(ctx).
		Model(&model.RoutingPolicy{}).
		Select("MAX(version)").
		Where("env = ? AND tenant_scope = ?", env, tenantScope).
		Scan(&maxVersion).Error
	if err != nil {
		return 0, err
	}
	if !maxVersion.Valid {
		return 1, nil
	}
	return uint32(maxVersion.Int64 + 1), nil
}

// CreateVersion persists a routing policy version. Caller should provide Version (e.g. via NextVersion).
func (r *RoutingPolicyRepository) CreateVersion(ctx context.Context, policy *model.RoutingPolicy) (*model.RoutingPolicy, error) {
	if policy == nil {
		return nil, errors.New("policy is nil")
	}
	if policy.Version == 0 {
		next, err := r.NextVersion(ctx, policy.Env, policy.TenantScope)
		if err != nil {
			return nil, err
		}
		policy.Version = next
	}
	if err := r.db.WithContext(ctx).Create(policy).Error; err != nil {
		return nil, err
	}
	return policy, nil
}

// Latest returns the latest version for a tenant scope (optionally filtered by status).
func (r *RoutingPolicyRepository) Latest(ctx context.Context, env, tenantScope, status string) (*model.RoutingPolicy, error) {
	query := r.db.WithContext(ctx).
		Where("env = ? AND tenant_scope = ?", env, tenantScope).
		Order("version DESC").
		Limit(1)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var record model.RoutingPolicy
	err := query.First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// UpdateStatus transitions a policy version to the specified status and optional payload updates.
func (r *RoutingPolicyRepository) UpdateStatus(ctx context.Context, env, tenantScope string, version uint32, status string, payload map[string]interface{}) error {
	updates := map[string]interface{}{
		"status": status,
	}
	for k, v := range payload {
		updates[k] = v
	}
	return r.db.WithContext(ctx).
		Model(&model.RoutingPolicy{}).
		Where("env = ? AND tenant_scope = ? AND version = ?", env, tenantScope, version).
		Updates(updates).Error
}

// FindVersion fetches a specific policy version.
func (r *RoutingPolicyRepository) FindVersion(ctx context.Context, env, tenantScope string, version uint32) (*model.RoutingPolicy, error) {
	var record model.RoutingPolicy
	err := r.db.WithContext(ctx).
		Where("env = ? AND tenant_scope = ? AND version = ?", env, tenantScope, version).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// ListByTenant returns most recent policies ordered by version desc.
func (r *RoutingPolicyRepository) ListByTenant(ctx context.Context, env, tenantScope string, limit int) ([]model.RoutingPolicy, error) {
	query := r.db.WithContext(ctx).
		Where("env = ? AND tenant_scope = ?", env, tenantScope).
		Order("version DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var records []model.RoutingPolicy
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
