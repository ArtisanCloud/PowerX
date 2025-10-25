// pkg/corex/db/persistence/repository/iam/api_key_repo.go
package iam

import (
	"context"
	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type APIKeyRepository struct {
	*repository.BaseRepository[dbm.APIKey]
	db *gorm.DB
}

func NewAPIKeyRepository(db *gorm.DB) *APIKeyRepository {
	return &APIKeyRepository{
		BaseRepository: repository.NewBaseRepository[dbm.APIKey](db),
		db:             db,
	}
}

func (r *APIKeyRepository) Create(ctx context.Context, k *dbm.APIKey) error {
	return r.db.WithContext(ctx).Create(k).Error
}

func (r *APIKeyRepository) FindByHash(ctx context.Context, hash string) (*dbm.APIKey, error) {
	var k dbm.APIKey
	if err := r.db.WithContext(ctx).Where("key_hash=? AND revoked_at_ms IS NULL", hash).First(&k).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *APIKeyRepository) Revoke(ctx context.Context, id uint64, ts int64) error {
	return r.db.WithContext(ctx).Model(&dbm.APIKey{}).
		Where("id=?", id).Update("revoked_at_ms", ts).Error
}

func (r *APIKeyRepository) TouchLastUsed(ctx context.Context, id uint64, ts int64) error {
	return r.db.WithContext(ctx).Model(&dbm.APIKey{}).
		Where("id=?", id).Update("last_used_at_ms", ts).Error
}
