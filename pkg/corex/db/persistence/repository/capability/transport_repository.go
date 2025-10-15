package capability

import (
	"context"
	"errors"

	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

// TransportProfileRepository 管理能力传输配置的增删改查。
type TransportProfileRepository struct {
	*repository.BaseRepository[capmodel.CapabilityTransportProfile]
	db *gorm.DB
}

// NewTransportProfileRepository 创建仓储实例。
func NewTransportProfileRepository(db *gorm.DB) *TransportProfileRepository {
	return &TransportProfileRepository{
		BaseRepository: repository.NewBaseRepository[capmodel.CapabilityTransportProfile](db),
		db:             db,
	}
}

// WithDB 返回绑定指定事务的新仓储。
func (r *TransportProfileRepository) WithDB(db *gorm.DB) *TransportProfileRepository {
	return &TransportProfileRepository{
		BaseRepository: repository.NewBaseRepository[capmodel.CapabilityTransportProfile](db),
		db:             db,
	}
}

// UpsertProfiles 以 (tenant_id, contract_id, transport) 作为唯一键批量写入。
func (r *TransportProfileRepository) UpsertProfiles(ctx context.Context, profiles []*capmodel.CapabilityTransportProfile) error {
	if len(profiles) == 0 {
		return nil
	}
	for _, profile := range profiles {
		var existing capmodel.CapabilityTransportProfile
		err := r.db.WithContext(ctx).
			Where("tenant_id = ? AND contract_id = ? AND transport = ?",
				profile.TenantID, profile.ContractID, profile.Transport).
			First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := r.db.WithContext(ctx).Create(profile).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		updates := map[string]interface{}{
			"mode":               profile.Mode,
			"timeout_millis":     profile.TimeoutMillis,
			"retry_policy":       profile.RetryPolicy,
			"streaming":          profile.Streaming,
			"qos":                profile.QoS,
			"endpoint_selector":  profile.EndpointSelector,
			"last_health_status": profile.LastHealthStatus,
			"status":             profile.Status,
		}
		if err := r.db.WithContext(ctx).Model(&existing).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListByContract 返回某个契约下的全部传输配置。
func (r *TransportProfileRepository) ListByContract(ctx context.Context, contractID uint64) ([]capmodel.CapabilityTransportProfile, error) {
	var profiles []capmodel.CapabilityTransportProfile
	err := r.db.WithContext(ctx).
		Where("contract_id = ?", contractID).
		Order("transport ASC").
		Find(&profiles).
		Error
	return profiles, err
}

// DeleteByContract 删除契约下所有传输配置。
func (r *TransportProfileRepository) DeleteByContract(ctx context.Context, contractID uint64) error {
	return r.db.WithContext(ctx).
		Unscoped().
		Where("contract_id = ?", contractID).
		Delete(&capmodel.CapabilityTransportProfile{}).
		Error
}
