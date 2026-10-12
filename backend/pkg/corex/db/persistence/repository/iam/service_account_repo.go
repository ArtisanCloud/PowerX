// pkg/corex/db/persistence/repository/iam/service_account_repo.go
package iam

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type APIKeyProfileRepository struct {
	*repository.BaseRepository[dbm.APIKeyProfile]
	db *gorm.DB
}

func NewAPIKeyProfileRepository(db *gorm.DB) *APIKeyProfileRepository {
	return &APIKeyProfileRepository{
		BaseRepository: repository.NewBaseRepository[dbm.APIKeyProfile](db),
		db:             db,
	}
}

// LockTenantProfile serializes permission updates, owner changes and rotation.
func (r *APIKeyProfileRepository) LockTenantProfile(ctx context.Context, tenantUUID string, profileID uint64) (*dbm.APIKeyProfile, error) {
	var item dbm.APIKeyProfile
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_uuid = ? AND id = ?", tenantUUID, profileID).First(&item).Error
	return &item, err
}

func (r *APIKeyProfileRepository) FindByKey(ctx context.Context, tenantUUID string, key string) (*dbm.APIKeyProfile, error) {
	var s dbm.APIKeyProfile
	if err := r.db.WithContext(ctx).Where("tenant_uuid=? AND key=?", tenantUUID, key).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *APIKeyProfileRepository) ListByTenant(ctx context.Context, tenantUUID string) ([]dbm.APIKeyProfile, error) {
	var items []dbm.APIKeyProfile
	query := r.db.WithContext(ctx).Model(&dbm.APIKeyProfile{})
	if strings.TrimSpace(tenantUUID) != "" {
		query = query.Where("tenant_uuid = ?", strings.TrimSpace(tenantUUID))
	}
	if err := query.Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
