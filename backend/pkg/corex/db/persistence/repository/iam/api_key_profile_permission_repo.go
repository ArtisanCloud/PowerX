package iam

import (
	"context"
	"time"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type APIKeyProfilePermissionRepository struct {
	*repository.BaseRepository[dbm.APIKeyProfilePermission]
	db *gorm.DB
}

func NewAPIKeyProfilePermissionRepository(db *gorm.DB) *APIKeyProfilePermissionRepository {
	return &APIKeyProfilePermissionRepository{
		BaseRepository: repository.NewBaseRepository[dbm.APIKeyProfilePermission](db),
		db:             db,
	}
}

func (r *APIKeyProfilePermissionRepository) ListPermissionIDsOfProfile(ctx context.Context, profileID uint64) ([]uint64, error) {
	var ids []uint64
	err := r.db.WithContext(ctx).
		Table((&dbm.APIKeyProfilePermission{}).GetTableName(true)).
		Where("profile_id = ?", profileID).
		Order("permission_id ASC").
		Pluck("permission_id", &ids).Error
	return ids, err
}

func (r *APIKeyProfilePermissionRepository) GrantByIDsTx(tx *gorm.DB, profileID uint64, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	rows := make([]dbm.APIKeyProfilePermission, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, dbm.APIKeyProfilePermission{
			ProfileID:    profileID,
			PermissionID: id,
			CreatedAt:    now,
		})
	}
	return tx.Table((&dbm.APIKeyProfilePermission{}).GetTableName(true)).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rows).Error
}

func (r *APIKeyProfilePermissionRepository) RevokeByIDsTx(tx *gorm.DB, profileID uint64, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	return tx.WithContext(tx.Statement.Context).
		Table((&dbm.APIKeyProfilePermission{}).GetTableName(true)).
		Where("profile_id = ? AND permission_id IN ?", profileID, ids).
		Delete(nil).Error
}
