// internal/api/system/user_handler.go
package system

import (
	"errors"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"gorm.io/gorm"
	"net/http"
	"strconv"
	"strings"
	"time"

	systemsvc "github.com/ArtisanCloud/PowerX/internal/service/system"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/gin-gonic/gin"
)

// ============= Handler（只依赖 Service） =============

type UserHandler struct {
	S *systemsvc.UserService
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{
		S: systemsvc.NewUserService(db),
	}
}

// ============= DTO（仅包含模型没有的扩展字段） =============

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
	UserName    *string     `json:"username,omitempty"`
}

type ListUsersReq struct {
	dto.PaginationRequest
	TenantID uint64 `form:"tenant_id" json:"tenant_id"`
	Keyword  string `form:"q"`
	Status   *int16 `form:"status"`
}

// 创建用户并把该用户加入租户时需要的扩展字段（模型字段直接来自 m.User）
type CreateSystemUserReq struct {
	m.User `json:",inline"`

	UserName        string   `json:"username" validate:"required,min=3,max=64"` // 在租户内的用户名
	TenantID        uint64   `json:"tenant_id" validate:"required,gt=0"`        // Root 选中的租户
	InitialPassword string   `json:"initial_password" validate:"omitempty,min=6,max=64"`
	DeptIDs         []uint64 `json:"dept_ids"` // 创建 member 时绑定的部门（可选）
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
	JTI string `json:"jti,omitempty"` // 推荐走 JTI 精确撤销
}

// 把已存在的 User 加入某个租户为 Member
type AddUserToTenantReq struct {
	TenantID uint64 `json:"tenant_id" validate:"required,gt=0"`
}

// ============= Handlers（全部转发给 Service） =============

// GET /api/admin/system/users
func (h *UserHandler) List(c *gin.Context) {
	var req ListUsersReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()

	ctx := c.Request.Context()
	// Service 需要提供：ListUsers(ctx, keyword, status, page, size, orderBy) ([]m.User, total, error)
	users, total, err := h.S.ListUsers(ctx, repoi.UserListFilter{
		TenantID: req.TenantID,
		Keyword:  req.Keyword,
		Status:   req.Status,
		Page:     req.Page,
		Size:     req.PageSize,
		OrderBy:  strings.TrimSpace(req.SortOrder),
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询用户失败", err)
		return
	}

	dto.ResponseList(c, users, &dto.PaginationResponse{Total: total, Page: req.Page, PageSize: req.PageSize})
}

// GET /api/admin/system/users/:id
func (h *UserHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ctx := c.Request.Context()

	// Service：GetUser(ctx, id) (*m.User, error)
	u, err := h.S.GetUser(ctx, id)
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "用户不存在或查询失败", err)
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

// POST /api/admin/system/users
// Root 视角：创建一个全局 User，并在指定租户创建一个 Member
func (h *UserHandler) Create(c *gin.Context) {
	var req CreateSystemUserReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	ctx := c.Request.Context()

	u := &m.User{
		Email:       strings.ToLower(strings.TrimSpace(req.Email)),
		Phone:       strings.TrimSpace(req.Phone),
		DisplayName: strings.TrimSpace(req.DisplayName),
		AvatarURL:   strings.TrimSpace(req.AvatarURL),
		Status:      utils.IfZeroInt16Ptr(&req.Status, 1),
		Meta:        req.Meta,
	}

	// Service：CreateSystemUser(ctx, user *m.User, tenantID uint64, username, initialPassword string, deptIDs []uint64) (userID uint64, err error)
	id, err := h.S.CreateSystemUser(ctx, u, req.TenantID, strings.ToLower(strings.TrimSpace(req.UserName)), strings.TrimSpace(req.InitialPassword), req.DeptIDs)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "创建失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"id": id})
}

// PATCH /api/admin/system/users/:id/add-to-tenant
// Root 视角：把已存在的 User 加入某个租户（创建 Member）
func (h *UserHandler) AddToTenant(c *gin.Context) {
	userID, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var req AddUserToTenantReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	memberID, err := h.S.AddUserToTenant(c.Request.Context(), userID, req.TenantID)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "加入租户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"member_id": memberID})
}

// PATCH /api/admin/system/users/:id
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
		updates["email"] = strings.ToLower(strings.TrimSpace(*req.Email))
	}
	if req.Phone != nil {
		updates["phone"] = strings.TrimSpace(*req.Phone)
	}
	if req.DisplayName != nil {
		updates["display_name"] = strings.TrimSpace(*req.DisplayName)
	}
	if req.AvatarURL != nil {
		updates["avatar_url"] = strings.TrimSpace(*req.AvatarURL)
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if len(updates) == 0 {
		dto.ResponseSuccess(c, gin.H{"ok": true})
		return
	}

	// Service：UpdateUser(ctx, id uint64, updates map[string]any) error
	if err := h.S.UpdateUser(ctx, id, updates); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新用户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// PUT /api/admin/system/users/:id/status
func (h *UserHandler) SetStatus(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req SetStatusReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	ctx := c.Request.Context()

	// Service：SetUserStatus(ctx, id uint64, status int16) error
	if err := h.S.SetUserStatus(ctx, id, req.Status); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新状态失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// DELETE /api/admin/system/users/:id
func (h *UserHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ctx := c.Request.Context()

	// Service：DeleteUser(ctx, id uint64) error
	if err := h.S.DeleteUser(ctx, id); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "删除用户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// PUT /api/admin/system/users/:id/restore
func (h *UserHandler) Restore(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ctx := c.Request.Context()

	// Service：RestoreUser(ctx, id uint64) error
	if err := h.S.RestoreUser(ctx, id); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "恢复用户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// POST /api/admin/system/users/:id/force-logout
func (h *UserHandler) ForceLogout(c *gin.Context) {
	_, _ = strconv.ParseUint(c.Param("id"), 10, 64) // 仅路径占位；真正按 JTI 撤销
	var req ForceLogoutReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	jti := strings.TrimSpace(req.JTI)
	if jti == "" {
		dto.ResponseError(c, http.StatusBadRequest, "强制下线失败", errors.New("missing jti"))
		return
	}

	// Service：ForceLogoutByJTI(ctx, jti string, nowMillis int64) error
	if err := h.S.ForceLogoutByJTI(c.Request.Context(), jti, time.Now().UnixMilli()); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "强制下线失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}
