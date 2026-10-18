package skills

import (
	"context"

	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	registrymodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

// ToolCapabilitySnapshot 是同一次读取的目录、租户合同及最新 registration；不是实现验真凭证。
type ToolCapabilitySnapshot struct {
	Record       registrymodel.CapabilityRecord
	Contract     capmodel.CapabilityContract
	Registration registrymodel.CapabilityRegistration
}

// ToolCatalogRepository 直接读取数据库，不使用可能滞后的工具/授权缓存。
type ToolCatalogRepository struct {
	*baseRepo.BaseRepository[registrymodel.CapabilityRecord]
	db *gorm.DB
}

// NewToolCatalogRepository 创建不使用目录缓存的预检仓储。
func NewToolCatalogRepository(db *gorm.DB) *ToolCatalogRepository {
	return &ToolCatalogRepository{BaseRepository: baseRepo.NewBaseRepository[registrymodel.CapabilityRecord](db), db: db}
}

// LoadToolCapability 读取给定能力及同租户合同，并保留最新 registration 的禁用状态。
func (r *ToolCatalogRepository) LoadToolCapability(ctx context.Context, tenantUUID, capabilityUUID, contractUUID string) (*ToolCapabilitySnapshot, error) {
	var out ToolCapabilitySnapshot
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("uuid = ?", capabilityUUID).First(&out.Record).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_uuid = ? AND contract_uuid = ?", tenantUUID, contractUUID).
			Preload("IOSchemas").Preload("TransportProfiles").First(&out.Contract).Error; err != nil {
			return err
		}
		// 不能查询任意旧 active 记录来掩盖最新版本的禁用或撤销。
		return tx.Where("tenant_uuid = ? AND capability_id = ?", tenantUUID, out.Record.CapabilityID).
			Order("version DESC").Preload("Adapters").First(&out.Registration).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
