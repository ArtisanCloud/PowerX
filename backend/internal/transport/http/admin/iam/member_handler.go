package iam

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"

	svc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
)

type MemberHandler struct {
	S *svc.MemberService
}

func NewMemberHandler(s *svc.MemberService) *MemberHandler {
	return &MemberHandler{S: s}
}

// -------- 请求结构（仅 Handler 层） --------

type ListMembersReq struct {
	dto.PaginationRequest
	Q         string  `form:"q"`
	Status    *int16  `form:"status"`
	DeptID    *uint64 `form:"dept_id"`
	Recursive bool    `form:"recursive,default=true"`
}
type CreateMemberReq struct {
	Member          m.Member `json:"member" validate:"required"`
	User            m.User   `json:"user"`
	DeptIDs         []uint64 `json:"dept_ids"`
	InitialPassword string   `json:"initial_password"`
}
type UpdateMemberReq struct {
	Member  *m.Member `json:"member"`
	User    *m.User   `json:"user"`
	DeptIDs *[]uint64 `json:"dept_ids"`
}
type SetMemberStatusReq struct {
	Status *int16 `json:"status" validate:"required"`
	Reason string `json:"reason"`
}
type PutMemberDepartmentsReq struct {
	DeptIDs []uint64 `json:"dept_ids"`
}
type ForceMemberLogoutReq struct {
	JTI string `json:"jti,omitempty"`
}

// -------- Handlers --------

// GET /api/v1/admin/iam/members
func (h *MemberHandler) List(c *gin.Context) {
	var req ListMembersReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()

	ctx := c.Request.Context()
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	items, total, err := h.S.ListMembersByTenantUUID(ctx, tenantUUID, svc.ListMembersOption{
		Page:      req.Page,
		PageSize:  req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
		Keyword:   req.Q,
		Status:    req.Status,
		DeptID:    req.DeptID,
		Recursive: req.Recursive,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}
	dto.ResponseList(c, items, &dto.PaginationResponse{Total: total, Page: req.Page, PageSize: req.PageSize})
}

// GET /api/v1/admin/iam/members/:id
func (h *MemberHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	res, err := h.S.GetMemberByTenantUUID(c.Request.Context(), tenantUUID, id)
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "member not found", err)
		return
	}
	dto.ResponseSuccess(c, res)
}

// POST /api/v1/admin/iam/members
func (h *MemberHandler) Create(c *gin.Context) {
	var req CreateMemberReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	id, err := h.S.CreateMemberByTenantUUID(c.Request.Context(), tenantUUID, svc.CreateMemberInput{
		Member:          req.Member,
		User:            req.User,
		DeptIDs:         req.DeptIDs,
		InitialPassword: req.InitialPassword,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "创建失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"id": id})
}

// PATCH /api/v1/admin/iam/members/:id
func (h *MemberHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req UpdateMemberReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	if err := h.S.UpdateMemberByTenantUUID(c.Request.Context(), tenantUUID, id, svc.UpdateMemberInput{
		Member:  req.Member,
		User:    req.User,
		DeptIDs: req.DeptIDs,
	}); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// PUT /api/v1/admin/iam/members/:id/status
func (h *MemberHandler) SetStatus(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req SetMemberStatusReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	if err := h.S.SetMemberStatusByTenantUUID(c.Request.Context(), tenantUUID, id, *req.Status, req.Reason); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "设置状态失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// DELETE /api/v1/admin/iam/members/:id
func (h *MemberHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	if err := h.S.DeleteMemberByTenantUUID(c.Request.Context(), tenantUUID, id); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "删除失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// PUT /api/v1/admin/iam/members/:id/restore
func (h *MemberHandler) Restore(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	if err := h.S.RestoreMemberByTenantUUID(c.Request.Context(), tenantUUID, id); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "恢复失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// PUT /api/v1/admin/iam/members/:id/departments
func (h *MemberHandler) PutDepartments(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req PutMemberDepartmentsReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	if err := h.S.PutMemberDepartmentsByTenantUUID(c.Request.Context(), tenantUUID, id, req.DeptIDs); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "设置失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// POST /api/v1/admin/iam/members/:id/force-logout
func (h *MemberHandler) ForceMemberLogout(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req ForceMemberLogoutReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	if err := h.S.ForceLogoutByTenantUUID(c.Request.Context(), tenantUUID, id, req.JTI); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "强制下线失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// 批量：?user_ids=1,2,3
type BatchGetMembersByUsersReq struct {
	UserIDs []uint64 `form:"user_ids,split" validate:"required,min=1,dive,gt=0"`
}

// 把已有 User 加入本租户（创建 member）
type AddExistingUserReq struct {
	UserName     string         `json:"username"      validate:"required,min=3,max=64"` // 本租户内唯一
	DisplayName  string         `json:"display_name"  validate:"omitempty,max=128"`
	AvatarURL    string         `json:"avatar_url"    validate:"omitempty,url"`
	Status       *int16         `json:"status"`        // 默认 1
	DeptID       *uint64        `json:"dept_id"`       // 主部门（可选）
	RoleIDs      []uint64       `json:"role_ids"`      // 赋角色（可选）
	Meta         datatypes.JSON `json:"meta"`          // 自定义（可选）
	InitPassword *string        `json:"init_password"` // 可选：若需立即可登录且无密码凭证
}

// ======== 新增：租户内视角 Handlers ========

// GET /api/v1/admin/iam/users/:user_id/member
func (h *MemberHandler) GetMemberByUser(c *gin.Context) {
	userID, _ := strconv.ParseUint(c.Param("user_id"), 10, 64)
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	mem, usr, err := h.S.GetMemberByUserTenantUUID(c.Request.Context(), tenantUUID, userID)
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "member not found in this tenant", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"member": mem,
		"user":   usr,
	})
}

// GET /api/v1/admin/iam/users/members?user_ids=1,2,3
// GET /api/v1/admin/iam/users/members?user_ids=1,2,3
func (h *MemberHandler) BatchGetMembersByUsers(c *gin.Context) {
	var req BatchGetMembersByUsersReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	members, usersByID, err := h.S.BatchGetMembersByUsersTenantUUID(c.Request.Context(), tenantUUID, req.UserIDs)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}

	// 组装输出：[{member, user}]
	out := make([]gin.H, 0, len(members))
	for i := range members {
		u := usersByID[members[i].UserID]
		out = append(out, gin.H{
			"member": members[i],
			"user":   u,
		})
	}
	dto.ResponseSuccess(c, gin.H{"items": out})
}

// POST /api/v1/admin/iam/users/:user_id/member
// POST /api/v1/admin/iam/users/:user_id/member
func (h *MemberHandler) AddExistingUser(c *gin.Context) {
	userID, _ := strconv.ParseUint(c.Param("user_id"), 10, 64)
	var req AddExistingUserReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	var meta datatypes.JSON
	if len(req.Meta) > 0 {
		_ = json.Unmarshal(req.Meta, &meta)
	}

	id, err := h.S.AddExistingUserAsMemberByTenantUUID(
		ctx,
		tenantUUID,
		userID,
		req.UserName,
		req.DisplayName,
		req.AvatarURL,
		req.Status,
		req.DeptID,
		req.RoleIDs,
		meta,
		req.InitPassword,
	)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "create member failed", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"id": id})
}
