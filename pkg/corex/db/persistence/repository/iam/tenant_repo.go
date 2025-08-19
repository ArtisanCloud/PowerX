package iam

import (
	"context"

	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type TenantRepository struct {
	*repository.BaseRepository[dbm.Tenant]
	db *gorm.DB
}

func NewTenantRepository(db *gorm.DB) *TenantRepository {
	return &TenantRepository{
		BaseRepository: repository.NewBaseRepository[dbm.Tenant](db),
		db:             db,
	}
}

func (r *TenantRepository) GetByID(ctx context.Context, id uint64) (*dbm.Tenant, error) {
	var t dbm.Tenant
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TenantRepository) GetByKey(ctx context.Context, key string) (*dbm.Tenant, error) {
	var t dbm.Tenant
	if err := r.db.WithContext(ctx).Where("key = ?", key).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TenantRepository) EnsureSystemTenant(ctx context.Context) (*dbm.Tenant, error) {
	t, err := r.GetByKey(ctx, dbm.SystemTenantKey)
	if err == nil && t != nil {
		return t, nil
	}
	sys := &dbm.Tenant{Key: dbm.SystemTenantKey, Name: "System", Status: 1}
	if err := r.db.WithContext(ctx).Create(sys).Error; err != nil {
		return nil, err
	}
	return sys, nil
}
