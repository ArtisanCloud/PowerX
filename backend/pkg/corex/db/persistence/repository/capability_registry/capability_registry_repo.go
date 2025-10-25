package capability_registry

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// CapabilityRegistryRepository 负责 CapabilityRegistration 的持久化访问。
type CapabilityRegistryRepository struct {
	*baseRepo.BaseRepository[models.CapabilityRegistration]
	db *gorm.DB
}

// NewCapabilityRegistryRepository 构造仓储实例。
func NewCapabilityRegistryRepository(db *gorm.DB) *CapabilityRegistryRepository {
	if db == nil {
		panic("capability registry repository requires non-nil db")
	}
	return &CapabilityRegistryRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.CapabilityRegistration](db),
		db:             db,
	}
}

func (r *CapabilityRegistryRepository) resolveDB(db *gorm.DB) *gorm.DB {
	if db != nil {
		return db
	}
	return r.db
}

// Create 创建新的能力注册快照（版本需从 1 开始）。
func (r *CapabilityRegistryRepository) Create(ctx context.Context, db *gorm.DB, reg Registration) (Registration, error) {
	return r.persist(ctx, r.resolveDB(db), reg, 0, true)
}

// Update 在乐观锁校验成功后生成新版本快照。
func (r *CapabilityRegistryRepository) Update(ctx context.Context, db *gorm.DB, reg Registration, expectedVersion uint64) (Registration, error) {
	if expectedVersion == 0 {
		return Registration{}, ErrVersionConflict
	}
	return r.persist(ctx, r.resolveDB(db), reg, expectedVersion, false)
}

// Disable 生成禁用版本快照，可选 version 校验。
func (r *CapabilityRegistryRepository) Disable(
	ctx context.Context,
	db *gorm.DB,
	capabilityID, tenantID, reason, actor string,
	expectedVersion uint64,
	next Registration,
) (Registration, error) {
	next.CapabilityID = capabilityID
	next.TenantID = tenantID
	next.DisableReason = reason
	next.UpdatedBy = actor
	return r.persist(ctx, r.resolveDB(db), next, expectedVersion, false)
}

// GetLatest 返回最新版本的能力注册快照。
func (r *CapabilityRegistryRepository) GetLatest(ctx context.Context, db *gorm.DB, capabilityID, tenantID string) (Registration, error) {
	resolved := r.resolveDB(db)
	var record models.CapabilityRegistration
	err := resolved.WithContext(ctx).
		Where("capability_id = ? AND tenant_id = ?", capabilityID, tenantID).
		Order("version DESC").
		Preload("RoutingPolicy").
		Preload("FallbackPlan").
		Preload("Adapters", func(db *gorm.DB) *gorm.DB {
			return db.Order("adapter_id ASC")
		}).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Registration{}, ErrNotFound
	}
	if err != nil {
		return Registration{}, err
	}
	return decodeRegistration(record)
}

// GetVersion 按版本号获取能力注册快照。
func (r *CapabilityRegistryRepository) GetVersion(ctx context.Context, db *gorm.DB, capabilityID, tenantID string, version uint64) (Registration, error) {
	resolved := r.resolveDB(db)
	var record models.CapabilityRegistration
	err := resolved.WithContext(ctx).
		Where("capability_id = ? AND tenant_id = ? AND version = ?", capabilityID, tenantID, version).
		Preload("RoutingPolicy").
		Preload("FallbackPlan").
		Preload("Adapters", func(db *gorm.DB) *gorm.DB {
			return db.Order("adapter_id ASC")
		}).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Registration{}, ErrNotFound
	}
	if err != nil {
		return Registration{}, err
	}
	return decodeRegistration(record)
}

// ListLatest 按租户分页返回最新版本快照。
func (r *CapabilityRegistryRepository) ListLatest(ctx context.Context, db *gorm.DB, tenantID string, limit, offset int) ([]Registration, int64, error) {
	resolved := r.resolveDB(db)
	base := resolved.WithContext(ctx).Model(&models.CapabilityRegistration{}).Where("tenant_id = ?", tenantID)

	var total int64
	if err := base.Distinct("capability_id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var capabilityIDs []string
	query := base.Select("DISTINCT capability_id").Order("capability_id ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	if err := query.Pluck("capability_id", &capabilityIDs).Error; err != nil {
		return nil, 0, err
	}

	result := make([]Registration, 0, len(capabilityIDs))
	for _, capabilityID := range capabilityIDs {
		item, err := r.GetLatest(ctx, resolved, capabilityID, tenantID)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, item)
	}
	return result, total, nil
}

func (r *CapabilityRegistryRepository) persist(ctx context.Context, db *gorm.DB, reg Registration, expectedVersion uint64, allowMissing bool) (Registration, error) {
	var out Registration
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var latest models.CapabilityRegistration
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("capability_id = ? AND tenant_id = ?", reg.CapabilityID, reg.TenantID).
			Order("version DESC").
			Limit(1).
			First(&latest).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			if !allowMissing && expectedVersion > 0 {
				return ErrVersionConflict
			}
		} else if err != nil {
			return err
		}

		currentVersion := uint64(0)
		if err == nil {
			currentVersion = latest.Version
		}
		if expectedVersion > 0 && currentVersion != expectedVersion {
			return ErrVersionConflict
		}

		nextVersion := currentVersion + 1
		if reg.Version > 0 {
			if reg.Version != nextVersion {
				return ErrVersionConflict
			}
		} else {
			reg.Version = nextVersion
		}

		envPolicies, err := encodeEnvironmentPolicies(reg.EnvironmentPolicies)
		if err != nil {
			return err
		}
		toolGrants, err := encodeToolGrantIDs(reg.ToolGrantIDs)
		if err != nil {
			return err
		}

		policyModel, err := encodeRoutingPolicy(reg.RoutingPolicy)
		if err != nil {
			return err
		}
		if err := tx.Create(policyModel).Error; err != nil {
			return err
		}

		fallbackModel, err := encodeFallbackPlan(reg.FallbackPlan, reg.CapabilityID)
		if err != nil {
			return err
		}
		if fallbackModel != nil {
			if err := tx.Create(fallbackModel).Error; err != nil {
				return err
			}
		}

		record := models.CapabilityRegistration{
			CapabilityID:        reg.CapabilityID,
			TenantID:            reg.TenantID,
			ContractRef:         reg.ContractRef,
			Status:              reg.Status,
			EnvironmentPolicies: envPolicies,
			ToolGrantIDs:        toolGrants,
			Version:             reg.Version,
			UpdatedBy:           reg.UpdatedBy,
			PublishedAt:         reg.PublishedAt,
			DisableReason:       reg.DisableReason,
			RoutingPolicyID:     policyModel.UUID,
		}
		if fallbackModel != nil {
			record.FallbackPlanID = &fallbackModel.UUID
		}

		if err := tx.Create(&record).Error; err != nil {
			return err
		}

		adapterModels, err := encodeAdapterEndpoints(reg, record.ID)
		if err != nil {
			return err
		}
		if len(adapterModels) > 0 {
			if err := tx.Create(&adapterModels).Error; err != nil {
				return err
			}
		}

		record.RoutingPolicy = policyModel
		if fallbackModel != nil {
			record.FallbackPlan = fallbackModel
		}
		record.Adapters = adapterModels

		decoded, err := decodeRegistration(record)
		if err != nil {
			return err
		}
		out = decoded
		return nil
	})
	if err != nil {
		return Registration{}, err
	}
	return out, nil
}

// DeleteStaleSnapshots 删除指定能力在 version 之前的快照。
func (r *CapabilityRegistryRepository) DeleteStaleSnapshots(ctx context.Context, db *gorm.DB, capabilityID, tenantID string, beforeVersion uint64) error {
	resolved := r.resolveDB(db)
	return resolved.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []uint64
		if err := tx.Model(&models.CapabilityRegistration{}).
			Where("capability_id = ? AND tenant_id = ? AND version < ?", capabilityID, tenantID, beforeVersion).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := tx.Where("registration_id IN ?", ids).Delete(&models.AdapterEndpoint{}).Error; err != nil {
			return err
		}
		if err := tx.Where("capability_id = ? AND tenant_id = ? AND version < ?", capabilityID, tenantID, beforeVersion).
			Delete(&models.CapabilityRegistration{}).Error; err != nil {
			return err
		}
		return nil
	})
}
