// internal/api/organization/user_handler.go
package iam

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
)

type UserHandler struct {
	UserRepo         *repoi.UserRepository
	MemberRepo       *repoi.MemberRepository
	MemberDeptRepo   *repoi.MemberDepartmentRepository
	CredRepo         *repoi.CredentialRepository
	RefreshTokenRepo *repoi.RefreshTokenRepository
	DB               *gorm.DB
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{
		UserRepo:         repoi.NewUserRepository(db),
		MemberRepo:       repoi.NewMemberRepository(db),
		MemberDeptRepo:   repoi.NewMemberDepartmentRepository(db),
		CredRepo:         repoi.NewCredentialRepository(db),
		RefreshTokenRepo: repoi.NewRefreshTokenRepository(db),
		DB:               db,
	}
}

// ------------------- DTO -------------------

type UserDTO struct {
	ID          uint64      `json:"id"`
	Email       string      `json:"email,omitempty"`
	Phone       string      `json:"phone,omitempty"`
	DisplayName string      `json:"display_name"`
	AvatarURL   string      `json:"avatar_url,omitempty"`
	Status      int16       `json:"status"`
	LastLoginAt *int64      `json:"last_login_at,omitempty"`
	Meta        interface{} `json:"meta,omitempty"`
	MemberID    *uint64     `json:"member_id,omitempty"`
	Username    *string     `json:"username,omitempty"`
}

type ListUsersReq struct {
	dto.PaginationRequest
	Keyword string `form:"q"`
	Status  *int16 `form:"status"`
}

type CreateUserReq struct {
	Username        string `json:"username" validate:"required"`
	Email           string `json:"email" validate:"omitempty,email"`
	Phone           string `json:"phone"`
	DisplayName     string `json:"display_name" validate:"required"`
	AvatarURL       string `json:"avatar_url"`
	Status          *int16 `json:"status"`
	InitialPassword string `json:"initial_password"`
}

type UpdateUserReq struct {
	Email       *string `json:"email" validate:"omitempty,email"`
	Phone       *string `json:"phone"`
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Status      *int16  `json:"status"`
}

type SetStatusReq struct {
	Status int16 `json:"status" validate:"required"`
}

type ForceLogoutReq struct {
	JTI string `json:"jti,omitempty"`
}

// ------------------- Handlers -------------------

// GET /admin/organization/users
func (h *UserHandler) List(c *gin.Context) {
	var req ListUsersReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()

	ctx := c.Request.Context()
	filter := repoi.UserListFilter{
		Keyword: req.Keyword,
		Status:  req.Status,
		Page:    req.Page,
		Size:    req.PageSize,
		OrderBy: req.SortBy + " " + req.SortOrder,
	}

	list, total, err := h.UserRepo.List(ctx, filter)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询用户失败", err)
		return
	}

	items := make([]UserDTO, 0, len(list))
	for _, u := range list {
		items = append(items, UserDTO{
			ID:          u.ID,
			Email:       u.Email,
			Phone:       u.Phone,
			DisplayName: u.DisplayName,
			AvatarURL:   u.AvatarURL,
			Status:      u.Status,
			LastLoginAt: u.LastLoginAt,
			Meta:        u.Meta,
		})
	}

	pagination := &dto.PaginationResponse{
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}
	dto.ResponseList(c, items, pagination)
}

// GET /admin/organization/users/:id
func (h *UserHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ctx := c.Request.Context()

	u, err := h.UserRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dto.ResponseError(c, http.StatusNotFound, "用户不存在", nil)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "查询用户失败", err)
		return
	}

	dto.ResponseSuccess(c, UserDTO{
		ID:          u.ID,
		Email:       u.Email,
		Phone:       u.Phone,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		Status:      u.Status,
		LastLoginAt: u.LastLoginAt,
		Meta:        u.Meta,
	})
}

// POST /admin/organization/users
func (h *UserHandler) Create(c *gin.Context) {
	var req CreateUserReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	ctx := c.Request.Context()
	u := &m.User{
		Email:       strings.ToLower(req.Email),
		Phone:       req.Phone,
		DisplayName: req.DisplayName,
		AvatarURL:   req.AvatarURL,
		Status:      1,
	}
	if req.Status != nil {
		u.Status = *req.Status
	}

	if err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(u).Error; err != nil {
			return err
		}
		mem := &m.Member{
			TenantID:    auth.GetTenantID(ctx),
			UserID:      u.ID,
			Username:    strings.ToLower(req.Username),
			DisplayName: req.DisplayName,
			AvatarURL:   req.AvatarURL,
			Status:      u.Status,
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(mem).Error
	}); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "创建用户失败", err)
		return
	}

	dto.ResponseSuccess(c, gin.H{"id": u.ID})
}

// PATCH /admin/organization/users/:id
func (h *UserHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req UpdateUserReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	ctx := c.Request.Context()
	updates := map[string]any{}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.AvatarURL != nil {
		updates["avatar_url"] = *req.AvatarURL
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if err := h.DB.WithContext(ctx).Model(&m.User{}).Where("id=?", id).Updates(updates).Error; err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新用户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// PUT /admin/organization/users/:id/status
func (h *UserHandler) SetStatus(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req SetStatusReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	ctx := c.Request.Context()
	if err := h.DB.WithContext(ctx).Model(&m.User{}).Where("id=?", id).Update("status", req.Status).Error; err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新状态失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// DELETE /admin/organization/users/:id
func (h *UserHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ctx := c.Request.Context()
	if err := h.DB.WithContext(ctx).Where("id=?", id).Delete(&m.User{}).Error; err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "删除用户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// PUT /admin/organization/users/:id/restore
func (h *UserHandler) Restore(c *gin.Context) {
	// 假设 User/Member 启用 gorm.DeletedAt
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ctx := c.Request.Context()
	if err := h.DB.WithContext(ctx).Unscoped().
		Model(&m.User{}).Where("id=?", id).Update("deleted_at", nil).Error; err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "恢复用户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// POST /admin/organization/users/:id/force-logout
func (h *UserHandler) ForceLogout(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req ForceLogoutReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	ctx := c.Request.Context()
	now := time.Now()
	if req.JTI != "" {
		if err := h.RefreshTokenRepo.RevokeByJTI(ctx, req.JTI, now.UnixMilli()); err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "强制下线失败", err)
			return
		}
	} else {
		if err := h.RefreshTokenRepo.RevokeAllForUser(ctx, id, auth.GetTenantID(ctx), now); err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "强制下线失败", err)
			return
		}
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}
