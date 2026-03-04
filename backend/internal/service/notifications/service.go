package notifications

import (
	"context"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/notification"
)

type Service struct {
	db   *gorm.DB
	repo *repo.NotificationRepository
	now  func() time.Time
}

func NewService(db *gorm.DB) *Service {
	return &Service{
		db:   db,
		repo: repo.NewNotificationRepository(db),
		now:  time.Now,
	}
}

type CreateInput struct {
	TenantUUID  string
	MemberUUID  string
	Title       string
	Content     string
	Type        string
	Category    string
	IsImportant bool
	RelatedID   string
	RelatedType string
	Actions     datatypes.JSON
	Metadata    datatypes.JSON
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*models.Notification, error) {
	tenantUUID := strings.ToLower(strings.TrimSpace(input.TenantUUID))
	if tenantUUID == "" {
		return nil, gorm.ErrInvalidData
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "系统通知"
	}
	content := strings.TrimSpace(input.Content)
	kind := strings.TrimSpace(input.Type)
	if kind == "" {
		kind = "info"
	}
	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = "system"
	}
	item := &models.Notification{
		TenantUUID:  tenantUUID,
		MemberUUID:  strings.TrimSpace(input.MemberUUID),
		Title:       title,
		Content:     content,
		Type:        kind,
		Category:    category,
		IsRead:      false,
		IsImportant: input.IsImportant,
		RelatedID:   strings.TrimSpace(input.RelatedID),
		RelatedType: strings.TrimSpace(input.RelatedType),
		Actions:     input.Actions,
		Metadata:    input.Metadata,
	}
	return s.repo.Create(ctx, item)
}

type ListInput struct {
	TenantUUID  string
	MemberUUID  string
	Category    string
	Type        string
	IsRead      *bool
	IsImportant *bool
	Page        int
	PageSize    int
}

func (s *Service) List(ctx context.Context, input ListInput) ([]models.Notification, int64, error) {
	return s.repo.List(ctx, repo.ListFilter{
		TenantUUID:  input.TenantUUID,
		MemberUUID:  input.MemberUUID,
		Category:    strings.TrimSpace(input.Category),
		Type:        strings.TrimSpace(input.Type),
		IsRead:      input.IsRead,
		IsImportant: input.IsImportant,
		Page:        input.Page,
		PageSize:    input.PageSize,
	})
}

func (s *Service) Get(ctx context.Context, tenantUUID, memberUUID, uuid string) (*models.Notification, error) {
	return s.repo.FindByUUID(ctx, tenantUUID, memberUUID, uuid)
}

func (s *Service) MarkRead(ctx context.Context, tenantUUID, memberUUID, uuid string) error {
	return s.repo.MarkRead(ctx, tenantUUID, memberUUID, uuid, s.now())
}

func (s *Service) Delete(ctx context.Context, tenantUUID, memberUUID, uuid string) error {
	return s.repo.Delete(ctx, tenantUUID, memberUUID, uuid)
}
