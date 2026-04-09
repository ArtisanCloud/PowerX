package notification

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type ListFilter struct {
	TenantUUID  string
	MemberUUID  string
	Category    string
	Type        string
	IsRead      *bool
	IsImportant *bool
	Page        int
	PageSize    int
}

// NotificationRepository 管理通知持久化。
type NotificationRepository struct {
	*baseRepo.BaseRepository[models.Notification]
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	if db == nil {
		panic("notification repository requires db")
	}
	return &NotificationRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.Notification](db),
		db:             db,
	}
}

func (r *NotificationRepository) List(ctx context.Context, filter ListFilter) ([]models.Notification, int64, error) {
	tenantUUID := strings.ToLower(strings.TrimSpace(filter.TenantUUID))
	memberUUID := strings.TrimSpace(filter.MemberUUID)
	if tenantUUID == "" {
		return nil, 0, gorm.ErrInvalidData
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

	q := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("tenant_uuid = ?", tenantUUID)
	if memberUUID != "" {
		q = q.Where("(member_uuid = ? OR member_uuid = '' OR member_uuid IS NULL)", memberUUID)
	}
	if filter.Category != "" {
		q = q.Where("category = ?", filter.Category)
	}
	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}
	if filter.IsRead != nil {
		q = q.Where("is_read = ?", *filter.IsRead)
	}
	if filter.IsImportant != nil {
		q = q.Where("is_important = ?", *filter.IsImportant)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.Notification
	if err := q.Order("created_at DESC, id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *NotificationRepository) FindByUUID(ctx context.Context, tenantUUID, memberUUID, rawUUID string) (*models.Notification, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	memberUUID = strings.TrimSpace(memberUUID)
	rawUUID = strings.TrimSpace(rawUUID)
	if tenantUUID == "" || rawUUID == "" {
		return nil, gorm.ErrInvalidData
	}
	parsed, err := uuid.Parse(rawUUID)
	if err != nil {
		return nil, gorm.ErrInvalidData
	}
	q := r.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, parsed)
	if memberUUID != "" {
		q = q.Where("member_uuid = ?", memberUUID)
	}
	var item models.Notification
	if err := q.Take(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *NotificationRepository) MarkRead(ctx context.Context, tenantUUID, memberUUID, rawUUID string, readAt time.Time) error {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	memberUUID = strings.TrimSpace(memberUUID)
	rawUUID = strings.TrimSpace(rawUUID)
	if tenantUUID == "" || rawUUID == "" {
		return gorm.ErrInvalidData
	}
	parsed, err := uuid.Parse(rawUUID)
	if err != nil {
		return gorm.ErrInvalidData
	}
	q := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("tenant_uuid = ? AND uuid = ?", tenantUUID, parsed)
	if memberUUID != "" {
		q = q.Where("member_uuid = ?", memberUUID)
	}
	return q.Updates(map[string]any{
		"is_read":    true,
		"read_at":    readAt,
		"updated_at": readAt,
	}).Error
}

func (r *NotificationRepository) Delete(ctx context.Context, tenantUUID, memberUUID, rawUUID string) error {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	memberUUID = strings.TrimSpace(memberUUID)
	rawUUID = strings.TrimSpace(rawUUID)
	if tenantUUID == "" || rawUUID == "" {
		return gorm.ErrInvalidData
	}
	parsed, err := uuid.Parse(rawUUID)
	if err != nil {
		return gorm.ErrInvalidData
	}
	q := r.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, parsed)
	if memberUUID != "" {
		q = q.Where("member_uuid = ?", memberUUID)
	}
	return q.Delete(&models.Notification{}).Error
}

func (r *NotificationRepository) DeleteAll(ctx context.Context, tenantUUID, memberUUID string) (int64, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	memberUUID = strings.TrimSpace(memberUUID)
	if tenantUUID == "" {
		return 0, gorm.ErrInvalidData
	}
	q := r.db.WithContext(ctx).Where("tenant_uuid = ?", tenantUUID)
	if memberUUID != "" {
		q = q.Where("(member_uuid = ? OR member_uuid = '' OR member_uuid IS NULL)", memberUUID)
	}
	tx := q.Delete(&models.Notification{})
	return tx.RowsAffected, tx.Error
}
