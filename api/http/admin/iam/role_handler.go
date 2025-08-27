package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
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

func NewRoleHandler(deps *bootstrap.Deps) *RoleHandler {
	return &RoleHandler{svc: iamsvc.NewRoleService(deps.DB)}
}

// Create：直接绑定到 dbm.Role，按约定修正/校验，再交由 Service
func (h *RoleHandler) Create(c *gin.Context) {
	var in dbm.Role
	if err := c.ShouldBindJSON(&in); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}

	in.Scope = strings.ToLower(strings.TrimSpace(in.Scope))
	switch iam.RoleScope(in.Scope) {
	case iam.RoleScopeTenant:
		// 租户角色：以 as_tenant_id 为准，避免跨租户创建
		if tid := auth.GetTenantID(c); &tid != nil && tid > 0 {
			in.TenantID = tid
		}
		if in.TenantID == 0 {
			dto.ResponseError(c, http.StatusBadRequest, "tenant 角色必须指定 tenant_id (>0)", nil)
			return
		}
	case iam.RoleScopeSystem:
		// 默认不允许通过普通 API 创建 system 角色（仅 seed/运维）
		dto.ResponseError(c, http.StatusForbidden, "禁止通过 API 创建 system 角色", nil)
		return
	default:
		dto.ResponseError(c, http.StatusBadRequest, "scope 仅支持 system|tenant", nil)
		return
	}

	out, err := h.svc.Create(c.Request.Context(), &in)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "创建角色失败", err)
		return
	}
	dto.ResponseSuccess(c, out)
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
