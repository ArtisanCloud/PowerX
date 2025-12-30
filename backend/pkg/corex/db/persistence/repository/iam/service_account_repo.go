// pkg/corex/db/persistence/repository/iam/service_account_repo.go
package iam

import (
	"context"
	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type ServiceAccountRepository struct {
	*repository.BaseRepository[dbm.ServiceAccount]
	db *gorm.DB
}

func NewServiceAccountRepository(db *gorm.DB) *ServiceAccountRepository {
	return &ServiceAccountRepository{
		BaseRepository: repository.NewBaseRepository[dbm.ServiceAccount](db),
		db:             db,
	}
}

func (r *ServiceAccountRepository) FindByKey(ctx context.Context, tenantUUID string, key string) (*dbm.ServiceAccount, error) {
	var s dbm.ServiceAccount
	if err := r.db.WithContext(ctx).Where("tenant_uuid=? AND key=?", tenantUUID, key).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}
