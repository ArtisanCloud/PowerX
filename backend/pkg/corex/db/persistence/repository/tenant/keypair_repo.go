// pkg/corex/db/persistence/repository/tenant/keypair_repo.go
package tenant

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"

	"gorm.io/gorm"
)

type TenantKeyPairRepository struct{ db *gorm.DB }

func NewTenantKeyPairRepository(db *gorm.DB) *TenantKeyPairRepository {
	return &TenantKeyPairRepository{db}
}

func (r *TenantKeyPairRepository) GetActiveByScope(ctx context.Context, env string, tenantUUID string) (*dbmodel.TenantKeyPair, error) {
	var kp dbmodel.TenantKeyPair
	err := r.db.WithContext(ctx).
		Where("env = ? AND tenant_uuid = ? AND active = ?", env, tenantUUID, true).
		First(&kp).Error
	if err != nil {
		return nil, err
	}
	return &kp, nil
}

func (r *TenantKeyPairRepository) Create(ctx context.Context, kp *dbmodel.TenantKeyPair) error {
	return r.db.WithContext(ctx).Create(kp).Error
}

func (r *TenantKeyPairRepository) DeactivateAll(ctx context.Context, env string, tenantUUID string) error {
	return r.db.WithContext(ctx).
		Model(&dbmodel.TenantKeyPair{}).
		Where("env = ? AND tenant_uuid = ?", env, tenantUUID).
		Update("active", false).Error
}
