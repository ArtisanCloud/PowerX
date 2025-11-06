package plugin_release

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// LocalInstallSessionRepository persists px-plugin dev/watch sessions.
type LocalInstallSessionRepository struct {
	*baseRepo.BaseRepository[models.LocalInstallSession]
	db *gorm.DB
}

// NewLocalInstallSessionRepository constructs the repository instance.
func NewLocalInstallSessionRepository(db *gorm.DB) *LocalInstallSessionRepository {
	if db == nil {
		panic("local install session repository requires non-nil db")
	}
	return &LocalInstallSessionRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.LocalInstallSession](db),
		db:             db,
	}
}

// CreateSession inserts a new session record.
func (r *LocalInstallSessionRepository) CreateSession(ctx context.Context, session *models.LocalInstallSession) (*models.LocalInstallSession, error) {
	if session == nil {
		return nil, gorm.ErrInvalidData
	}
	return r.BaseRepository.Create(ctx, session)
}

// UpdateSessionStatus updates status, log pointers and expiration.
func (r *LocalInstallSessionRepository) UpdateSessionStatus(ctx context.Context, sessionUUID uuid.UUID, status string, logPointers interface{}, expiredAt *time.Time) error {
	if sessionUUID == uuid.Nil {
		return gorm.ErrInvalidData
	}
	update := map[string]interface{}{
		"status": status,
	}
	if logPointers != nil {
		update["log_pointers"] = logPointers
	}
	if expiredAt != nil {
		update["expired_at"] = *expiredAt
	}
	return r.db.WithContext(ctx).
		Model(&models.LocalInstallSession{}).
		Where("uuid = ?", sessionUUID).
		Updates(update).Error
}

// GetActiveSession fetches the active session for a developer within a tenant.
func (r *LocalInstallSessionRepository) GetActiveSession(ctx context.Context, tenantID, developerID uint64) (*models.LocalInstallSession, error) {
	var session models.LocalInstallSession
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND developer_id = ? AND status = ?", tenantID, developerID, models.LocalInstallStatusInProgress).
		Order("created_at DESC").
		Take(&session).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// CleanupExpired removes sessions whose expiration is in the past.
func (r *LocalInstallSessionRepository) CleanupExpired(ctx context.Context, now time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("expired_at IS NOT NULL AND expired_at < ?", now).
		Delete(&models.LocalInstallSession{})
	return result.RowsAffected, result.Error
}
