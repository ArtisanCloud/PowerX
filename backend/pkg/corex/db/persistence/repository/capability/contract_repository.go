package capability

import (
	"context"
	"errors"

	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

// ContractRepository 封装 Capability 契约的持久化读写。
type ContractRepository struct {
	*repository.BaseRepository[capmodel.CapabilityContract]
	db *gorm.DB
}

// NewContractRepository 创建契约仓储实例。
func NewContractRepository(db *gorm.DB) *ContractRepository {
	return &ContractRepository{
		BaseRepository: repository.NewBaseRepository[capmodel.CapabilityContract](db),
		db:             db,
	}
}

// WithDB 返回绑定指定事务的新仓储。
func (r *ContractRepository) WithDB(db *gorm.DB) *ContractRepository {
	return &ContractRepository{
		BaseRepository: repository.NewBaseRepository[capmodel.CapabilityContract](db),
		db:             db,
	}
}

// UpsertContract 根据 (tenant_id, capability_key, version) 唯一键插入或更新契约主体。
func (r *ContractRepository) UpsertContract(ctx context.Context, contract *capmodel.CapabilityContract) (*capmodel.CapabilityContract, error) {
	var existing capmodel.CapabilityContract
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND capability_key = ? AND version = ?", contract.TenantID, contract.CapabilityKey, contract.Version).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := r.db.WithContext(ctx).Create(contract).Error; err != nil {
			return nil, err
		}
		return contract, nil
	}
	if err != nil {
		return nil, err
	}

	update := map[string]interface{}{
		"provider_id":            contract.ProviderID,
		"display_name":           contract.DisplayName,
		"description":            contract.Description,
		"security_scope":         contract.SecurityScope,
		"tool_grant_required":    contract.ToolGrantRequired,
		"lifecycle_state":        contract.LifecycleState,
		"observability_config":   contract.ObservabilityConfig,
		"transport_preferences":  contract.TransportPreferences,
		"status":                 contract.Status,
		"effective_at":           contract.EffectiveAt,
		"deprecated_at":          contract.DeprecatedAt,
		"replacement_capability": contract.ReplacementCapability,
		"updated_by":             contract.UpdatedBy,
	}
	if err := r.db.WithContext(ctx).Model(&existing).Updates(update).Error; err != nil {
		return nil, err
	}
	existing.ProviderID = contract.ProviderID
	existing.DisplayName = contract.DisplayName
	existing.Description = contract.Description
	existing.SecurityScope = contract.SecurityScope
	existing.ToolGrantRequired = contract.ToolGrantRequired
	existing.LifecycleState = contract.LifecycleState
	existing.ObservabilityConfig = contract.ObservabilityConfig
	existing.TransportPreferences = contract.TransportPreferences
	existing.Status = contract.Status
	existing.EffectiveAt = contract.EffectiveAt
	existing.DeprecatedAt = contract.DeprecatedAt
	existing.ReplacementCapability = contract.ReplacementCapability
	existing.UpdatedBy = contract.UpdatedBy
	return &existing, nil
}

// FindByKeyVersion 获取单个契约，optionally 预加载关联。
func (r *ContractRepository) FindByKeyVersion(ctx context.Context, tenantID uint64, capabilityKey, version string, preload bool) (*capmodel.CapabilityContract, error) {
	if capabilityKey == "" || version == "" {
		return nil, errors.New("capability key/version 不能为空")
	}

	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND capability_key = ? AND version = ?", tenantID, capabilityKey, version)

	if preload {
		query = query.
			Preload("IOSchemas").
			Preload("TransportProfiles").
			Preload("ErrorBindings").
			Preload("ErrorBindings.ErrorTaxonomy")
	}

	var contract capmodel.CapabilityContract
	if err := query.First(&contract).Error; err != nil {
		return nil, err
	}

	return &contract, nil
}

// ReplaceIOSchemas 以事务方式替换契约的 IO Schema 描述。
func (r *ContractRepository) ReplaceIOSchemas(ctx context.Context, contractID uint64, schemas []*capmodel.CapabilityIOSchema) error {
	db := r.db.WithContext(ctx)

	if err := db.Where("contract_id = ?", contractID).Delete(&capmodel.CapabilityIOSchema{}).Error; err != nil {
		return err
	}

	if len(schemas) > 0 {
		if err := db.Create(&schemas).Error; err != nil {
			return err
		}
	}
	return nil
}

// ReplaceErrorBindings 维护契约与错误条目的关联。
func (r *ContractRepository) ReplaceErrorBindings(ctx context.Context, contractID uint64, bindings []*capmodel.CapabilityContractErrorTaxonomy) error {
	db := r.db.WithContext(ctx)

	if err := db.Where("contract_id = ?", contractID).Delete(&capmodel.CapabilityContractErrorTaxonomy{}).Error; err != nil {
		return err
	}

	if len(bindings) > 0 {
		if err := db.Create(&bindings).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListContracts 按租户与关键字简单分页查询。
func (r *ContractRepository) ListContracts(ctx context.Context, tenantID uint64, keyword string, limit, offset int) ([]capmodel.CapabilityContract, int64, error) {
	query := r.db.WithContext(ctx).Model(&capmodel.CapabilityContract{}).
		Where("tenant_id = ?", tenantID)

	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("capability_key ILIKE ? OR display_name ILIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var contracts []capmodel.CapabilityContract
	if err := query.Order("updated_at DESC").Find(&contracts).Error; err != nil {
		return nil, 0, err
	}

	return contracts, total, nil
}
