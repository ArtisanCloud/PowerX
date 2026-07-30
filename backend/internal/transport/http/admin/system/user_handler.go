// internal/api/system/user_handler.go
package system

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svciam "github.com/ArtisanCloud/PowerX/internal/service/iam"
	systemsvc "github.com/ArtisanCloud/PowerX/internal/service/system"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/gin-gonic/gin"
)

// ============= Handler（只依赖 Service） =============

type UserHandler struct {
	S          *systemsvc.UserService
	tenantRepo *tenantrepo.TenantRepository
}

func (h *UserHandler) requireUserIDFromUUIDParam(c *gin.Context) (uint64, bool) {
	userID, err := h.S.ResolveUserIDByUUID(c.Request.Context(), c.Param("user_uuid"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "缺少有效用户参数", err)
		return 0, false
	}
	return userID, true
}

func NewUserHandler(deps *shared.Deps) *UserHandler {
	repo := tenantRepoFromDeps(deps)
	return &UserHandler{
		S:          systemsvc.NewUserService(deps.DB),
		tenantRepo: repo,
	}
}

func tenantRepoFromDeps(deps *shared.Deps) *tenantrepo.TenantRepository {
	if deps == nil {
		return nil
	}
	if deps.TenantSvc != nil && deps.TenantSvc.Repo != nil {
		return deps.TenantSvc.Repo
	}
	if deps.DB != nil {
		return tenantrepo.NewTenantRepository(deps.DB)
	}
	return nil
}

// ============= DTO（仅包含模型没有的扩展字段） =============

type UserDTO struct {
	UUID        string      `json:"uuid"`
	Email       string      `json:"email,omitempty"`
	Phone       string      `json:"phone,omitempty"`
	DisplayName string      `json:"display_name"`
	AvatarURL   string      `json:"avatar_url,omitempty"`
	Status      int16       `json:"status"`
	LastLoginAt *int64      `json:"last_login_at,omitempty"`
	Meta        interface{} `json:"meta,omitempty"`
	MemberUUID  *string     `json:"member_uuid,omitempty"`
	UserName    *string     `json:"username,omitempty"`
}

type MemberDTO struct {
	UUID        string      `json:"uuid"`
	TenantUUID  string      `json:"tenant_uuid"`
	UserUUID    string      `json:"user_uuid"`
	UserName    string      `json:"username"`
	DisplayName string      `json:"display_name"`
	AvatarURL   string      `json:"avatar_url,omitempty"`
	Status      int16       `json:"status"`
	Meta        interface{} `json:"meta,omitempty"`
}

type MemberWithProfileDTO struct {
	Member  *MemberDTO `json:"Member"`
	User    *UserDTO   `json:"User"`
	DeptIDs []uint64   `json:"DeptIDs,omitempty"`
}

func toMemberWithProfileDTOs(items []svciam.MemberWithProfile) []MemberWithProfileDTO {
	out := make([]MemberWithProfileDTO, 0, len(items))
	for i := range items {
		out = append(out, toMemberWithProfileDTO(items[i]))
	}
	return out
}

func toMemberWithProfileDTO(item svciam.MemberWithProfile) MemberWithProfileDTO {
	var userDTO *UserDTO
	if item.User != nil {
		userDTO = &UserDTO{
			UUID:        item.User.UUID.String(),
			Email:       item.User.Email,
			Phone:       item.User.Phone,
			DisplayName: item.User.DisplayName,
			AvatarURL:   item.User.AvatarURL,
			Status:      item.User.Status,
			LastLoginAt: item.User.LastLoginAt,
			Meta:        item.User.Meta,
		}
	}

	var memberDTO *MemberDTO
	if item.Member != nil {
		memberUUID := item.Member.UUID.String()
		memberDTO = &MemberDTO{
			UUID:        memberUUID,
			TenantUUID:  item.Member.TenantUUID,
			UserUUID:    item.Member.UserUUID,
			UserName:    item.Member.Username,
			DisplayName: item.Member.DisplayName,
			AvatarURL:   item.Member.AvatarURL,
			Status:      item.Member.Status,
			Meta:        item.Member.Meta,
		}
		if userDTO != nil {
			userDTO.MemberUUID = &memberUUID
			userDTO.UserName = &item.Member.Username
		}
	}

	return MemberWithProfileDTO{
		Member:  memberDTO,
		User:    userDTO,
		DeptIDs: item.DeptIDs,
	}
}

type ListUsersReq struct {
	dto.PaginationRequest
	TenantUUID string `form:"tenant_uuid" validate:"required"`
	Keyword    string `form:"q"`
	Status     *int16 `form:"status"`
}

// 创建用户并把该用户加入租户时需要的扩展字段（模型字段直接来自 m.User）
type CreateSystemUserReq struct {
	m.User `json:",inline"`

	TenantUUID      string   `json:"tenant_uuid" validate:"required"`
	UserName        string   `json:"username" validate:"required,min=3,max=64"` // 在租户内的用户名
	InitialPassword string   `json:"initial_password" validate:"omitempty,min=6,max=64"`
	DeptIDs         []uint64 `json:"dept_ids"`   // 创建 member 时绑定的部门（可选）
	RoleUUIDs       []string `json:"role_uuids"` // 创建 member 时绑定的角色（可选，空则默认 role_user）
}

type UpdateUserReq struct {
	TenantUUID  string  `json:"tenant_uuid" validate:"required"`
	UserName    *string `json:"username" validate:"omitempty,min=3,max=64"`
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

type ResetPasswordReq struct {
	NewPassword string `json:"new_password" validate:"required,min=6,max=64"`
}

// 把已存在的 User 加入某个租户为 Member
type AddUserToTenantReq struct {
}

type SetUserRolesReq struct {
	TenantUUID string   `json:"tenant_uuid" validate:"required"`
	RoleUUIDs  []string `json:"role_uuids"`
}

type ListUserRolesReq struct {
	TenantUUID string `form:"tenant_uuid" validate:"required"`
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

	tenantCtx, ok := requireTenantContextByUUID(c, h.tenantRepo, req.TenantUUID)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	// Service 需要提供：ListUsers(ctx, keyword, status, page, size, orderBy) ([]m.User, total, error)
	users, total, err := h.S.ListUsers(ctx, repoi.UserListFilter{
		TenantUUID: tenantCtx.UUID(),
		Keyword:    req.Keyword,
		Status:     req.Status,
		Page:       req.Page,
		Size:       req.PageSize,
		OrderBy:    strings.TrimSpace(req.SortOrder),
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询用户失败", err)
		return
	}

	dto.ResponseList(c, toMemberWithProfileDTOs(users), &dto.PaginationResponse{Total: total, Page: req.Page, PageSize: req.PageSize})
}

// GET /api/admin/system/users/:user_uuid
func (h *UserHandler) Get(c *gin.Context) {
	id, ok := h.requireUserIDFromUUIDParam(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// Service：GetUser(ctx, id) (*m.User, error)
	u, err := h.S.GetUser(ctx, id)
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "用户不存在或查询失败", err)
		return
	}
	dto.ResponseSuccess(c, UserDTO{
		UUID:        u.UUID.String(),
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
	tenantCtx, ok := requireTenantContextByUUID(c, h.tenantRepo, req.TenantUUID)
	if !ok {
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

	userUUID, err := h.S.CreateSystemUser(
		ctx,
		u,
		tenantCtx.UUID(),
		strings.ToLower(strings.TrimSpace(req.UserName)),
		strings.TrimSpace(req.InitialPassword),
		req.DeptIDs,
		req.RoleUUIDs,
	)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "创建失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"user_uuid": userUUID})
}

// PATCH /api/admin/system/users/:user_uuid/add-to-tenant
// Root 视角：把已存在的 User 加入某个租户（创建 Member）
func (h *UserHandler) AddToTenant(c *gin.Context) {
	userID, ok := h.requireUserIDFromUUIDParam(c)
	if !ok {
		return
	}

	var req AddUserToTenantReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	tenantCtx, ok := requireTenantContext(c, h.tenantRepo)
	if !ok {
		return
	}

	memberUUID, err := h.S.AddUserToTenant(c.Request.Context(), userID, tenantCtx.UUID())
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "加入租户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"member_uuid": memberUUID})
}

// PATCH /api/admin/system/users/:user_uuid
func (h *UserHandler) Update(c *gin.Context) {
	id, ok := h.requireUserIDFromUUIDParam(c)
	if !ok {
		return
	}
	var req UpdateUserReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantCtx, ok := requireTenantContextByUUID(c, h.tenantRepo, req.TenantUUID)
	if !ok {
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
	if len(updates) == 0 && req.UserName == nil {
		dto.ResponseSuccess(c, gin.H{"ok": true})
		return
	}

	// Service：UpdateUserInTenant(ctx, id, tenant_uuid, updates, username) error
	if err := h.S.UpdateUserInTenant(ctx, id, tenantCtx.UUID(), updates, req.UserName); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新用户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// PUT /api/admin/system/users/:user_uuid/status
func (h *UserHandler) SetStatus(c *gin.Context) {
	id, ok := h.requireUserIDFromUUIDParam(c)
	if !ok {
		return
	}
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

// DELETE /api/admin/system/users/:user_uuid
func (h *UserHandler) Delete(c *gin.Context) {
	id, ok := h.requireUserIDFromUUIDParam(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// Service：DeleteUser(ctx, id uint64) error
	if err := h.S.DeleteUser(ctx, id); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "删除用户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// PUT /api/admin/system/users/:user_uuid/restore
func (h *UserHandler) Restore(c *gin.Context) {
	id, ok := h.requireUserIDFromUUIDParam(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// Service：RestoreUser(ctx, id uint64) error
	if err := h.S.RestoreUser(ctx, id); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "恢复用户失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// POST /api/admin/system/users/:user_uuid/force-logout
func (h *UserHandler) ForceLogout(c *gin.Context) {
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

// PUT /api/admin/system/users/:user_uuid/password
func (h *UserHandler) ResetPassword(c *gin.Context) {
	id, ok := h.requireUserIDFromUUIDParam(c)
	if !ok {
		return
	}
	var req ResetPasswordReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if err := h.S.ResetUserPassword(c.Request.Context(), id, req.NewPassword); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "重置密码失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// GET /api/admin/system/users/:user_uuid/roles
func (h *UserHandler) ListRoles(c *gin.Context) {
	id, ok := h.requireUserIDFromUUIDParam(c)
	if !ok {
		return
	}
	var req ListUserRolesReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantCtx, ok := requireTenantContextByUUID(c, h.tenantRepo, req.TenantUUID)
	if !ok {
		return
	}
	roleUUIDs, err := h.S.ListUserRoleUUIDs(c.Request.Context(), id, tenantCtx.UUID())
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "查询用户角色失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"role_uuids": roleUUIDs})
}

// PUT /api/admin/system/users/:user_uuid/roles
func (h *UserHandler) SetRoles(c *gin.Context) {
	id, ok := h.requireUserIDFromUUIDParam(c)
	if !ok {
		return
	}
	var req SetUserRolesReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantCtx, ok := requireTenantContextByUUID(c, h.tenantRepo, req.TenantUUID)
	if !ok {
		return
	}
	if err := h.S.SetUserRoleUUIDs(c.Request.Context(), id, tenantCtx.UUID(), req.RoleUUIDs); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "设置用户角色失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}
