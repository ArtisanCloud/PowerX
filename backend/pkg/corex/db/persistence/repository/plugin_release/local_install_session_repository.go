package plugin_release

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
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

// GetSessionByUUID returns a session by its UUID (nil when not found).
func (r *LocalInstallSessionRepository) GetSessionByUUID(ctx context.Context, sessionUUID uuid.UUID) (*models.LocalInstallSession, error) {
	if sessionUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	type sessionRow struct {
		ID           uint64
		UUID         string
		TenantID     uint64
		DeveloperID  uint64
		ArtifactURI  string
		Status       string
		LogPointers  datatypes.JSON
		FeatureFlags datatypes.JSON
		CreatedAt    time.Time
		UpdatedAt    time.Time
		ExpiredAt    string
	}

	var row sessionRow
	err := r.db.WithContext(ctx).
		Table(models.LocalInstallSession{}.TableName()).
		Select("id", "uuid", "tenant_id", "developer_id", "artifact_uri", "status", "log_pointers", "feature_flags", "created_at", "updated_at", "expired_at").
		Where("uuid = ?", sessionUUID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	uid, err := uuid.Parse(row.UUID)
	if err != nil {
		return nil, err
	}

	session := &models.LocalInstallSession{
		PowerUUIDModel: coremodel.PowerUUIDModel{
			ID:        row.ID,
			UUID:      uid,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		},
		TenantID:     row.TenantID,
		DeveloperID:  row.DeveloperID,
		ArtifactURI:  row.ArtifactURI,
		Status:       row.Status,
		LogPointers:  row.LogPointers,
		FeatureFlags: row.FeatureFlags,
	}
	if ts := strings.TrimSpace(row.ExpiredAt); ts != "" {
		if parsed, err := parseTimestamp(ts); err == nil {
			session.ExpiredAt = &parsed
		}
	}
	return session, nil
}

// UpdateLogPointers overwrites log pointers snapshot.
func (r *LocalInstallSessionRepository) UpdateLogPointers(ctx context.Context, sessionUUID uuid.UUID, logPointers datatypes.JSON) error {
	if sessionUUID == uuid.Nil {
		return gorm.ErrInvalidData
	}
	return r.db.WithContext(ctx).
		Model(&models.LocalInstallSession{}).
		Where("uuid = ?", sessionUUID).
		Update("log_pointers", logPointers).Error
}

func parseTimestamp(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp format: %s", value)
}
