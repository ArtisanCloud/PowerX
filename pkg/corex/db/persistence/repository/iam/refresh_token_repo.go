package iam

import (
	"context"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"time"

	"gorm.io/gorm"
)

type RefreshTokenRepository struct {
	*repository.BaseRepository[dbm.RefreshToken]
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{
		BaseRepository: repository.NewBaseRepository[dbm.RefreshToken](db),
		db:             db,
	}
}

func (r *RefreshTokenRepository) Issue(ctx context.Context, rt *dbm.RefreshToken) error {
	return r.db.WithContext(ctx).Create(rt).Error
}

func (r *RefreshTokenRepository) GetByJTI(ctx context.Context, jti string) (*dbm.RefreshToken, error) {
	var rt dbm.RefreshToken
	if err := r.db.WithContext(ctx).Where("jti = ?", jti).First(&rt).Error; err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *RefreshTokenRepository) RevokeByJTI(ctx context.Context, jti string, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&dbm.RefreshToken{}).
		Where("jti = ? AND revoked_at IS NULL", jti).
		Update("revoked_at", at).Error
}

func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID, tenantID uint64, before time.Time) error {
	return r.db.WithContext(ctx).
		Model(&dbm.RefreshToken{}).
		Where("user_id = ? AND tenant_id = ? AND expires_at > ? AND revoked_at IS NULL", userID, tenantID, before).
		Update("revoked_at", before).Error
}

func (r *RefreshTokenRepository) CleanupExpired(ctx context.Context, before time.Time) (int64, error) {
	tx := r.db.WithContext(ctx).Where("expires_at < ?", before).Delete(&dbm.RefreshToken{})
	return tx.RowsAffected, tx.Error
}
