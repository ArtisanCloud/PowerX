package notifications

import (
	"context"
	"errors"
	"strings"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreateStorageAlertOnce 只供部署层指定的主机告警使用；固定 UUID 防止响应丢失后重复通知。
func (s *Service) CreateStorageAlertOnce(ctx context.Context, input CreateInput) (*models.Notification, error) {
	if cap.ServiceCredential(ctx) || input.RelatedType != "storage_capacity" || len(input.RelatedID) < 1 || len(input.RelatedID) > 128 || input.MemberUUID == "" {
		return nil, gorm.ErrInvalidData
	}
	if err := NewRecipientAccess(s.db).Require(ctx, input.TenantUUID, input.MemberUUID); err != nil {
		return nil, err
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("powerx:storage:"+strings.ToLower(input.TenantUUID)+":"+strings.ToLower(input.MemberUUID)+":"+input.RelatedID))
	if existing, err := s.repo.FindStorageAlert(ctx, input.TenantUUID, input.MemberUUID, id); err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	input.storageUUID = id
	item, err := s.Create(ctx, input)
	if err == nil {
		return item, nil
	}
	// 唯一键同时保护两个 worker 的重试；只读回同租户、同成员的固定用途通知。
	if existing, readErr := s.repo.FindStorageAlert(ctx, input.TenantUUID, input.MemberUUID, id); readErr == nil {
		return existing, nil
	}
	return nil, err
}
