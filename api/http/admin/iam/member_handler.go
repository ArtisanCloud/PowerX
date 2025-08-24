package iam

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	svc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
)

type MemberHandler struct {
	S *svc.MemberService
}

func NewMemberHandler(s *svc.MemberService) *MemberHandler { return &MemberHandler{S: s} }

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
	Status int16  `json:"status" validate:"required"`
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
	tid := auth.GetTenantID(ctx)
	items, total, err := h.S.ListMembers(ctx, svc.ListMembersOption{
		Page:      req.Page,
		PageSize:  req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
		TenantID:  tid,
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
	ctx := c.Request.Context()
	tid := auth.GetTenantID(ctx)

	res, err := h.S.GetMember(ctx, tid, id)
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
	ctx := c.Request.Context()
	tid := auth.GetTenantID(ctx)

	id, err := h.S.CreateMember(ctx, tid, svc.CreateMemberInput{
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
	ctx := c.Request.Context()
	tid := auth.GetTenantID(ctx)

	if err := h.S.UpdateMember(ctx, tid, id, svc.UpdateMemberInput{
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
	ctx := c.Request.Context()
	tid := auth.GetTenantID(ctx)

	if err := h.S.SetMemberStatus(ctx, tid, id, req.Status, req.Reason); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "设置状态失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// DELETE /api/v1/admin/iam/members/:id
func (h *MemberHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ctx := c.Request.Context()
	tid := auth.GetTenantID(ctx)

	if err := h.S.DeleteMember(ctx, tid, id); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "删除失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// PUT /api/v1/admin/iam/members/:id/restore
func (h *MemberHandler) Restore(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ctx := c.Request.Context()
	tid := auth.GetTenantID(ctx)

	if err := h.S.RestoreMember(ctx, tid, id); err != nil {
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
	ctx := c.Request.Context()
	tid := auth.GetTenantID(ctx)

	if err := h.S.PutMemberDepartments(ctx, tid, id, req.DeptIDs); err != nil {
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
	ctx := c.Request.Context()
	tid := auth.GetTenantID(ctx)

	if err := h.S.ForceLogout(ctx, tid, id, req.JTI); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "强制下线失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}
