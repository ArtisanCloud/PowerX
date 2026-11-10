package notification

import (
	"context"
	"errors"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"gorm.io/gorm"
	"time"
)

var ErrRevisionConflict = errors.New("notification revision conflict")
var ErrCardNotOwned = errors.New("notification card not owned")

func (r *NotificationRepository) InvalidateCard(ctx context.Context, tenant, owner, member, id string, source int64, now time.Time, reopen bool, attachMetadata []byte, expectedSource ...int64) (*models.Notification, bool, error) {
	var row models.Notification
	query := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&models.Notification{}).Where("tenant_uuid = ? AND service_owner = ? AND member_uuid = ? AND uuid = ?", tenant, owner, member, id)
	}
	if err := query().First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrCardNotOwned
		}
		return nil, false, err
	}
	if row.SourceRevision == 0 && len(attachMetadata) == 0 {
		return nil, false, ErrCardNotOwned
	}
	expected := int64(0)
	if len(expectedSource) > 0 {
		expected = expectedSource[0]
	}
	if len(attachMetadata) > 0 && row.SourceRevision != expected {
		return nil, false, ErrRevisionConflict
	}
	updates := map[string]any{"source_revision": source, "revision": gorm.Expr("revision + 1"), "updated_at": now}
	q := query()
	if len(attachMetadata) > 0 {
		updates["metadata"] = string(attachMetadata)
		q = q.Where("source_revision = ?", expected)
	}
	if reopen {
		updates["is_read"] = false
		updates["read_at"] = nil
	}
	tx := q.Where("source_revision < ?", source).Updates(updates)
	if tx.Error != nil {
		return nil, false, tx.Error
	}
	if len(attachMetadata) > 0 && tx.RowsAffected == 0 {
		return nil, false, ErrRevisionConflict
	}
	var current models.Notification
	if err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND service_owner = ? AND member_uuid = ? AND uuid = ?", tenant, owner, member, id).First(&current).Error; err != nil {
		return nil, false, err
	}
	return &current, tx.RowsAffected > 0, nil
}

// CAS prevents a receipt from an older card from acknowledging a later update.
func (r *NotificationRepository) MarkReadVersion(ctx context.Context, tenant, member, id string, expected int64, now time.Time) error {
	if tenant == "" || member == "" || id == "" {
		return gorm.ErrInvalidData
	}
	q := r.db.WithContext(ctx).Model(&models.Notification{}).Where("tenant_uuid = ? AND member_uuid = ? AND uuid = ?", tenant, member, id)
	if expected > 0 {
		q = q.Where("revision = ?", expected)
	} else {
		q = q.Where("source_revision = ?", expected)
	}
	tx := q.Updates(map[string]any{"is_read": true, "read_at": now, "updated_at": now})
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		// Legacy tenant broadcasts have no shared read mutation: their member-scoped
		// receipt remains client-side. Never mark the broadcast read for everyone.
		var count int64
		broadcast := r.db.WithContext(ctx).Model(&models.Notification{}).Where("tenant_uuid = ? AND uuid = ? AND source_revision = 0 AND (member_uuid = '' OR member_uuid IS NULL)", tenant, id)
		if expected > 0 {
			broadcast = broadcast.Where("revision = ?", expected)
		}
		if err := broadcast.Count(&count).Error; err != nil {
			return err
		}
		if count == 1 {
			return nil
		}
		return ErrRevisionConflict
	}
	return nil
}

func (r *NotificationRepository) FindOwned(ctx context.Context, tenant, owner, member, id string) (*models.Notification, error) {
	var row models.Notification
	if err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND service_owner = ? AND member_uuid = ? AND uuid = ?", tenant, owner, member, id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCardNotOwned
		}
		return nil, err
	}
	return &row, nil
}
