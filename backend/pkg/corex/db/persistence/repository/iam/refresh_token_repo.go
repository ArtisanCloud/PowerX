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

func (r *RefreshTokenRepository) RevokeByJTI(ctx context.Context, jti string, revokedAtMs int64) error {
	return r.db.WithContext(ctx).
		Model(&dbm.RefreshToken{}).
		Where("jti = ? AND revoked_at IS NULL", jti).
		Update("revoked_at", revokedAtMs).Error
}

// 批量：按 user_uuid + tenant_uuid 撤销该用户在该租户下的所有 refresh
func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userUUID, tenantUUID string, revokedAtMs int64) error {
	return r.db.WithContext(ctx).
		Model(&dbm.RefreshToken{}).
		Where("user_uuid = ? AND tenant_uuid = ? AND revoked_at IS NULL", userUUID, tenantUUID).
		Updates(map[string]any{"revoked_at": revokedAtMs}).Error
}
func (r *RefreshTokenRepository) CleanupExpired(ctx context.Context, before time.Time) (int64, error) {
	tx := r.db.WithContext(ctx).Where("expires_at < ?", before).Delete(&dbm.RefreshToken{})
	return tx.RowsAffected, tx.Error
}
