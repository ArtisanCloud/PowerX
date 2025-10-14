package capability

import (
	"context"

	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// UpsertProfiles 以 (tenant_id, contract_id, transport) 作为唯一键批量写入。
func (r *TransportProfileRepository) UpsertProfiles(ctx context.Context, profiles []*capmodel.CapabilityTransportProfile) error {
	if len(profiles) == 0 {
		return nil
	}
	unique := []clause.Column{
		{Name: "tenant_id"},
		{Name: "contract_id"},
		{Name: "transport"},
	}
	_, err := r.BaseRepository.UpsertBatch(ctx, profiles, unique)
	return err
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
		Where("contract_id = ?", contractID).
		Delete(&capmodel.CapabilityTransportProfile{}).
		Error
}
