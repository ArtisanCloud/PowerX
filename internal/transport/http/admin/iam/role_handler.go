package iam

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	// 统一响应/校验
)

// 列表查询参数（仅查询用，不重复定义实体）
type RolelistQuery struct {
	Scope    string  `form:"scope"`           // system|tenant（为空则按 as_tenant_id 推断）
	TenantID *uint64 `form:"tenant_id"`       // 可被 as_tenant_id 覆盖
	Keyword  string  `form:"keyword"`         // code/name 模糊
	Builtin  *bool   `form:"builtin"`         // true/false
	Page     int     `form:"page,default=1"`  // 页码
	Size     int     `form:"size,default=20"` // 每页
	Sort     string  `form:"sort"`            // 如 "name asc" / "created_at desc"
}

type RoleHandler struct{ svc *iamsvc.RoleService }

func NewRoleHandler(deps *shared.Deps) *RoleHandler {
	return &RoleHandler{svc: iamsvc.NewRoleService(deps.DB)}
}

// POST /api/admin/iam/roles
type roleCreateReq struct {
	Scope       string   `json:"scope"        binding:"required,oneof=system tenant"`
	TenantID    uint64   `json:"tenant_id"`
	Code        string   `json:"code"         binding:"required"`
	Name        string   `json:"name"         binding:"required"`
	Description string   `json:"description"`
	Builtin     bool     `json:"builtin"`
	PermIDs     []uint64 `json:"perm_ids"` // 可选
}

func (h *RoleHandler) Create(c *gin.Context) {
	var req roleCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}

	role := &dbm.Role{
		Scope:       string(iam.RoleScopeTenant),
		TenantID:    req.TenantID,
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		Builtin:     req.Builtin,
	}

	out, bindRes, err := h.svc.CreateWithPerms(c.Request.Context(), role, req.PermIDs)
	if err != nil {
		// Service 内已区分 forbidden/invalid 等错误，这里直接 400/403 的细分可按需再加
		dto.ResponseError(c, http.StatusBadRequest, "创建角色失败", err)
		return
	}

	// 返回角色以及（如有）权限设置结果，便于前端调试/提示
	dto.ResponseSuccess(c, gin.H{
		"role": out,
		"perm": bindRes, // {added,removed,now,skipped_deprecated}；若 req.PermIDs 为空为 nil
	})
}

type updateRolePayload struct {
	Name        string `json:"name" binding:"required,max=128"`
	Description string `json:"description" binding:"max=2000"`
}

func (h *RoleHandler) Update(c *gin.Context) {
	var in updateRolePayload
	if err := c.ShouldBindJSON(&in); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}

	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	tid := auth.GetTenantID(c)

	patch := &dbm.Role{
		Name:        strings.TrimSpace(in.Name),
		Description: strings.TrimSpace(in.Description),
	}

	if err := h.svc.Update(c.Request.Context(), roleID, &tid, patch); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新角色失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"updated": true})
}

func (h *RoleHandler) Delete(c *gin.Context) {
	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	tid := auth.GetTenantID(c)
	if err := h.svc.Delete(c.Request.Context(), roleID, &tid); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		dto.ResponseError(c, status, "删除角色失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"deleted": true})
}

func (h *RoleHandler) Get(c *gin.Context) {
	roleID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	tid := auth.GetTenantID(c)
	m, err := h.svc.Get(c.Request.Context(), roleID, tid)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "查询角色失败", err)
		return
	}
	if m == nil {
		dto.ResponseError(c, http.StatusNotFound, "角色不存在", nil)
		return
	}
	dto.ResponseSuccess(c, m)
}

func (h *RoleHandler) List(c *gin.Context) {
	var q RolelistQuery
	// 这里直接用 ShouldBindQuery；若你要更严格的校验，可用 dto.ValidateRequestWithContext
	if err := c.ShouldBindQuery(&q); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	// as_tenant_id 优先
	if tid := auth.GetTenantID(c); tid > 0 {
		q.TenantID = &tid
	}

	page, err := h.svc.List(c.Request.Context(), iamsvc.ListOpt{
		TenantID: q.TenantID,
		Scope:    q.Scope,
		Keyword:  q.Keyword,
		Builtin:  q.Builtin,
		Page:     q.Page,
		Size:     q.Size,
		Sort:     q.Sort,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "查询角色列表失败", err)
		return
	}

	// 统一分页响应
	pg := &dto.PaginationResponse{
		Total:    page.Total,
		Page:     page.PageIndex,
		PageSize: page.PageSize,
	}
	dto.ResponseList(c, page.List, pg)
}
