package iam

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repoiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	repotenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RootSupportSessionService struct {
	*service.BaseService
	userRepo   *repoiam.UserRepository
	tenantRepo *repotenant.TenantRepository
	clock      func() time.Time
}

type StartRootSupportSessionInput struct {
	TargetTenantUUID string
	Reason           string
	Mode             string
}

func NewRootSupportSessionService(db *gorm.DB) *RootSupportSessionService {
	return &RootSupportSessionService{
		BaseService: &service.BaseService{DB: db},
		userRepo:    repoiam.NewUserRepository(db),
		tenantRepo:  repotenant.NewTenantRepository(db),
		clock:       time.Now,
	}
}

func (s *RootSupportSessionService) Start(ctx context.Context, in StartRootSupportSessionInput) (*modeliam.RootSupportSession, error) {
	if s == nil || s.DB == nil {
		return nil, dto.NewInternal("root support session service 未初始化", nil)
	}
	rootUserID := reqctx.GetUserID(ctx)
	if rootUserID == 0 {
		return nil, dto.NewUnauthorized("未登录", nil)
	}
	isRoot, err := s.userRepo.IsRootUser(ctx, rootUserID)
	if err != nil {
		return nil, dto.NewInternal("校验 root 身份失败", err)
	}
	if !isRoot {
		return nil, dto.NewErrorWithCode(http.StatusForbidden, "ROOT_SUPPORT_ROOT_REQUIRED", "仅 root 可创建支持会话", nil)
	}

	targetTenantUUID, err := normalizeSupportTenantUUID(in.TargetTenantUUID)
	if err != nil {
		return nil, dto.NewBadRequest("target_tenant_uuid 必须是合法 UUID", err)
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, dto.NewBadRequest("reason 不能为空", nil)
	}
	mode := normalizeSupportMode(in.Mode)
	if mode == "" {
		return nil, dto.NewBadRequest("mode 必须是 read_only 或 write_enabled", nil)
	}
	if _, err := s.tenantRepo.GetByUUID(ctx, targetTenantUUID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, dto.NewErrorWithCode(http.StatusNotFound, "ROOT_SUPPORT_TARGET_TENANT_NOT_FOUND", "目标租户不存在", err)
		}
		return nil, dto.NewInternal("查询目标租户失败", err)
	}

	now := s.clock().UTC()
	item := &modeliam.RootSupportSession{
		RootUserID:       rootUserID,
		TargetTenantUUID: targetTenantUUID,
		Reason:           reason,
		Mode:             mode,
		Status:           modeliam.RootSupportSessionStatusActive,
		StartedAt:        now,
	}
	if err := s.DB.WithContext(ctx).Create(item).Error; err != nil {
		return nil, dto.NewInternal("创建 root support session 失败", err)
	}
	return item, nil
}

func (s *RootSupportSessionService) End(ctx context.Context, id uint64) (*modeliam.RootSupportSession, error) {
	if s == nil || s.DB == nil {
		return nil, dto.NewInternal("root support session service 未初始化", nil)
	}
	if id == 0 {
		return nil, dto.NewBadRequest("support session id required", nil)
	}
	rootUserID := reqctx.GetUserID(ctx)
	if rootUserID == 0 {
		return nil, dto.NewUnauthorized("未登录", nil)
	}
	isRoot, err := s.userRepo.IsRootUser(ctx, rootUserID)
	if err != nil {
		return nil, dto.NewInternal("校验 root 身份失败", err)
	}
	if !isRoot {
		return nil, dto.NewErrorWithCode(http.StatusForbidden, "ROOT_SUPPORT_ROOT_REQUIRED", "仅 root 可结束支持会话", nil)
	}

	var item modeliam.RootSupportSession
	err = s.DB.WithContext(ctx).
		Where("id = ? AND root_user_id = ?", id, rootUserID).
		First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dto.NewErrorWithCode(http.StatusNotFound, "ROOT_SUPPORT_SESSION_NOT_FOUND", "root support session 不存在", err)
	}
	if err != nil {
		return nil, dto.NewInternal("查询 root support session 失败", err)
	}
	if item.Status != modeliam.RootSupportSessionStatusActive {
		return &item, nil
	}
	endedAt := s.clock().UTC()
	item.Status = modeliam.RootSupportSessionStatusEnded
	item.EndedAt = &endedAt
	if err := s.DB.WithContext(ctx).Save(&item).Error; err != nil {
		return nil, dto.NewInternal("结束 root support session 失败", err)
	}
	return &item, nil
}

func normalizeSupportTenantUUID(raw string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func normalizeSupportMode(raw string) string {
	mode := strings.TrimSpace(raw)
	if mode == "" {
		return modeliam.RootSupportSessionModeReadOnly
	}
	switch mode {
	case modeliam.RootSupportSessionModeReadOnly, modeliam.RootSupportSessionModeWriteEnabled:
		return mode
	default:
		return ""
	}
}
