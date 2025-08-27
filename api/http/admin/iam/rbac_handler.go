package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
)

// ---- DTOs（最小化，仅此文件内使用） ----
type grantIDsReq struct {
	PermIDs []uint64 `json:"perm_ids" binding:"required,min=1"`
}
type grantTriplesReq struct {
	Items []struct {
		Plugin   string `json:"plugin" binding:"required"`
		Resource string `json:"resource" binding:"required"`
		Action   string `json:"action" binding:"required"`
	} `json:"items" binding:"required,min=1"`
}
type bindMemberReq struct {
	TenantID uint64 `json:"tenant_id" binding:"required"`
	MemberID uint64 `json:"member_id" binding:"required"`
}

type RBACHandler struct{ svc *iamsvc.RBACService }

func NewRBACHandler(deps *bootstrap.Deps) *RBACHandler {
	return &RBACHandler{svc: iamsvc.NewRBACService(deps.DB)}
}

// 角色授予权限（通过权限ID列表）
// POST /api/v1/admin/iam/roles/:id/permissions/grant-ids
func (h *RBACHandler) GrantByIDs(c *gin.Context) {
	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req grantIDsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	if err := h.svc.GrantPermsByIDs(c.Request.Context(), roleID, req.PermIDs); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "授予权限失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"granted": len(req.PermIDs)})
}

// 角色授予权限（通过 triples：plugin/resource/action）
// POST /api/v1/admin/iam/roles/:id/permissions/grant
func (h *RBACHandler) GrantByTriples(c *gin.Context) {
	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req grantTriplesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	ts := make([]iamsvc.PermTriple, 0, len(req.Items))
	for _, it := range req.Items {
		ts = append(ts, iamsvc.PermTriple{Plugin: it.Plugin, Resource: it.Resource, Action: it.Action})
	}
	if err := h.svc.GrantPermsByTriples(c.Request.Context(), roleID, ts); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "授予权限失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"granted": len(ts)})
}

// 角色撤销权限（通过权限ID列表）
// POST /api/v1/admin/iam/roles/:id/permissions/revoke-ids
func (h *RBACHandler) RevokeByIDs(c *gin.Context) {
	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req grantIDsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	if err := h.svc.RevokePermissionsFromRole(c.Request.Context(), roleID, req.PermIDs); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "撤销权限失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"revoked": len(req.PermIDs)})
}

// 列出角色已授予的权限
// GET /api/v1/admin/iam/roles/:id/permissions
func (h *RBACHandler) ListPermsOfRole(c *gin.Context) {
	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	perms, err := h.svc.ListPermsOfRole(c.Request.Context(), roleID)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "查询角色权限失败", err)
		return
	}
	dto.ResponseList(c, perms, nil)
}

// 将角色绑定到成员（直绑）
// POST /api/v1/admin/iam/roles/:id/bind/member
func (h *RBACHandler) BindRoleToMember(c *gin.Context) {
	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req bindMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	if err := h.svc.BindRoleToMember(c.Request.Context(), req.TenantID, roleID, req.MemberID); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "绑定角色失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"bound": true})
}

// 解除成员的一个角色绑定（按绑定记录ID）
// DELETE /api/v1/admin/iam/roles/:id/bindings/:binding_id
func (h *RBACHandler) UnbindRoleFromMember(c *gin.Context) {
	bindingID, _ := strconv.ParseUint(c.Param("binding_id"), 10, 64)
	tenantID, _ := strconv.ParseUint(c.Query("tenant_id"), 10, 64) // 非 root 必须是本租户
	if err := h.svc.UnbindRoleFromMember(c.Request.Context(), tenantID, bindingID); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "解绑角色失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"unbound": true})
}

// 自测：鉴权检查（当前登录人）
// GET /api/v1/admin/iam/me/check?plugin=iam&resource=role&action=read
func (h *RBACHandler) CheckPermission(c *gin.Context) {
	plugin := c.Query("plugin")
	resource := c.Query("resource")
	action := c.Query("action")

	tenantID, _ := strconv.ParseUint(c.Query("tenant_id"), 10, 64) // 可选，不传用上下文
	memberID, _ := strconv.ParseUint(c.Query("member_id"), 10, 64) // 可选，不传用上下文

	ok, err := h.svc.Enforce(c.Request.Context(), tenantID, memberID, plugin, resource, action)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "鉴权检查失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": ok})
}
