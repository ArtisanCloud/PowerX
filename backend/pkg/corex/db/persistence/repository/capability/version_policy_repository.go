package capability

import (
	"context"
	"errors"

	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

// VersionPolicyRepository 封装 Capability 版本策略的持久化逻辑。
type VersionPolicyRepository struct {
	*repository.BaseRepository[capmodel.CapabilityVersionPolicy]
	db *gorm.DB
}

// NewVersionPolicyRepository 创建仓储实例。
func NewVersionPolicyRepository(db *gorm.DB) *VersionPolicyRepository {
	return &VersionPolicyRepository{
		BaseRepository: repository.NewBaseRepository[capmodel.CapabilityVersionPolicy](db),
		db:             db,
	}
}

// WithDB 返回绑定指定事务的新仓储。
func (r *VersionPolicyRepository) WithDB(db *gorm.DB) *VersionPolicyRepository {
	return &VersionPolicyRepository{
		BaseRepository: repository.NewBaseRepository[capmodel.CapabilityVersionPolicy](db),
		db:             db,
	}
}

// GetByKey 根据租户与能力 Key 获取版本策略。
func (r *VersionPolicyRepository) GetByKey(ctx context.Context, tenantUUID string, capabilityKey string) (*capmodel.CapabilityVersionPolicy, error) {
	var policy capmodel.CapabilityVersionPolicy
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND capability_key = ?", tenantUUID, capabilityKey).
		First(&policy).Error
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

// UpsertPolicy 插入或更新版本策略。
func (r *VersionPolicyRepository) UpsertPolicy(ctx context.Context, policy *capmodel.CapabilityVersionPolicy) (*capmodel.CapabilityVersionPolicy, error) {
	var existing capmodel.CapabilityVersionPolicy
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND capability_key = ?", policy.TenantUUID, policy.CapabilityKey).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := r.db.WithContext(ctx).Create(policy).Error; err != nil {
			return nil, err
		}
		return policy, nil
	}
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{
		"default_strategy":     policy.DefaultStrategy,
		"allowed_versions":     policy.AllowedVersions,
		"compatibility_matrix": policy.CompatibilityMatrix,
		"deprecation_policy":   policy.DeprecationPolicy,
		"audit_config":         policy.AuditConfig,
		"status":               policy.Status,
	}
	if err := r.db.WithContext(ctx).Model(&existing).Updates(updates).Error; err != nil {
		return nil, err
	}
	existing.DefaultStrategy = policy.DefaultStrategy
	existing.AllowedVersions = policy.AllowedVersions
	existing.CompatibilityMatrix = policy.CompatibilityMatrix
	existing.DeprecationPolicy = policy.DeprecationPolicy
	existing.AuditConfig = policy.AuditConfig
	existing.Status = policy.Status
	return &existing, nil
}
