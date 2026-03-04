package iam

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// ---- DTOs（最小化，仅此文件内使用） ----
type grantIDsReq struct {
	PermIDs []uint64 `json:"perm_ids" binding:"required,min=1"`
}
type grantTriplesReq struct {
	Items []struct {
		Module   string `json:"module"`
		Plugin   string `json:"plugin"`
		Resource string `json:"resource" binding:"required"`
		Action   string `json:"action" binding:"required"`
	} `json:"items" binding:"required,min=1"`
}
type bindMemberReq struct {
	MemberID uint64 `json:"member_id" binding:"required"`
}

type RBACHandler struct {
	svc *iamsvc.RBACService
}

func NewRBACHandler(deps *shared.Deps) *RBACHandler {
	return &RBACHandler{
		svc: iamsvc.NewRBACService(deps.DB),
	}
}

func (h *RBACHandler) actorContextFromGin(c *gin.Context) (iamsvc.ActorContext, bool) {
	ctx := c.Request.Context()
	actor := iamsvc.ActorContext{
		IsRoot: reqctx.IsRoot(ctx),
	}
	if actor.IsRoot {
		if uuid := strings.TrimSpace(reqctx.TenantUUIDFromGin(c)); uuid != "" {
			canonical, err := reqctx.CanonicalTenantUUID(uuid)
			if err != nil {
				dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid must be valid", err)
				return iamsvc.ActorContext{}, false
			}
			actor.TenantUUID = canonical
		}
		return actor, true
	}
	uuid, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return iamsvc.ActorContext{}, false
	}
	actor.TenantUUID = uuid
	return actor, true
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
	actor, ok := h.actorContextFromGin(c)
	if !ok {
		return
	}
	if err := h.svc.GrantPermsByIDs(c.Request.Context(), actor, roleID, req.PermIDs); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "授予权限失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"granted": len(req.PermIDs)})
}

// 角色授予权限（通过 triples：module/resource/action）
// POST /api/v1/admin/iam/roles/:id/permissions/grant
func (h *RBACHandler) GrantByTriples(c *gin.Context) {
	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req grantTriplesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	actor, ok := h.actorContextFromGin(c)
	if !ok {
		return
	}
	ts := make([]iamsvc.PermTriple, 0, len(req.Items))
	for _, it := range req.Items {
		module := strings.TrimSpace(it.Module)
		if module == "" {
			module = strings.TrimSpace(it.Plugin)
		}
		if module == "" {
			dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", errors.New("module is required"))
			return
		}
		ts = append(ts, iamsvc.PermTriple{Module: module, Resource: it.Resource, Action: it.Action})
	}
	if err := h.svc.GrantPermsByTriples(c.Request.Context(), actor, roleID, ts); err != nil {
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
	actor, ok := h.actorContextFromGin(c)
	if !ok {
		return
	}
	if err := h.svc.RevokePermissionsFromRole(c.Request.Context(), actor, roleID, req.PermIDs); err != nil {
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
	actor, ok := h.actorContextFromGin(c)
	if !ok {
		return
	}
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	if err := h.svc.BindRoleToMember(c.Request.Context(), actor, tenantUUID, roleID, req.MemberID); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "绑定角色失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"bound": true})
}

// 解除成员的一个角色绑定（按绑定记录ID）
// DELETE /api/v1/admin/iam/roles/:id/bindings/:binding_id
func (h *RBACHandler) UnbindRoleFromMember(c *gin.Context) {
	bindingID, _ := strconv.ParseUint(c.Param("binding_id"), 10, 64)
	actor, ok := h.actorContextFromGin(c)
	if !ok {
		return
	}
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	if err := h.svc.UnbindRoleFromMember(c.Request.Context(), actor, tenantUUID, bindingID); err != nil {
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

	actor, ok := h.actorContextFromGin(c)
	if !ok {
		return
	}
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	memberID, _ := strconv.ParseUint(c.Query("member_id"), 10, 64) // 可选，不传用上下文

	pass, err := h.svc.Enforce(c.Request.Context(), actor, tenantUUID, memberID, plugin, resource, action)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "鉴权检查失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": pass})
}

type setIDsReq struct {
	IDs []uint64 `json:"ids"`
}

func (h *RBACHandler) ListPermIDs(c *gin.Context) {
	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	actor, ok := h.actorContextFromGin(c)
	if !ok {
		return
	}
	ids, err := h.svc.ListPermissionIDs(c.Request.Context(), actor, roleID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrForbidden) {
			status = http.StatusForbidden
		}
		if errors.Is(err, service.ErrRoleNotFound) {
			status = http.StatusNotFound
		}
		dto.ResponseError(c, status, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ids": ids})
}

// PUT /admin/iam/roles/:id/permissions/set-ids
func (h *RBACHandler) SetPermIDs(c *gin.Context) {
	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		IDs []uint64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	actor, ok := h.actorContextFromGin(c)
	if !ok {
		return
	}
	res, err := h.svc.SetPermissionIDs(c.Request.Context(), actor, roleID, req.IDs)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrForbidden) {
			status = http.StatusForbidden
		}
		if errors.Is(err, service.ErrRoleNotFound) {
			status = http.StatusNotFound
		}
		dto.ResponseError(c, status, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, res)
}
