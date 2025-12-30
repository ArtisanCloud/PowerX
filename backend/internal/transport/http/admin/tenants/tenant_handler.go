// api/http/admin/tenants/tenant_handler.go
package tenants

import (
	mdltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	svc "github.com/ArtisanCloud/PowerX/internal/service/tenant"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
)

// -------- Handler --------

type TenantHandler struct {
	S *svc.TenantService
}

func NewTenantHandler(s *svc.TenantService) *TenantHandler { return &TenantHandler{S: s} }

// -------- 请求结构（仅 Handler 层） --------

type ListTenantsReq struct {
	dto.PaginationRequest
	Q         string  `form:"q"`          // 模糊 name/domain
	Status    *string `form:"status"`     // 可选：active/inactive/suspended...
	Plan      *string `form:"plan"`       // 可选：套餐过滤
	SortBy    string  `form:"sort_by"`    // 默认 created_at
	SortOrder string  `form:"sort_order"` // 默认 desc
}

// GET /api/v1/admin/tenants
func (h *TenantHandler) ListTenants(c *gin.Context) {
	var req ListTenantsReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	// 兜底分页与排序
	req.SetDefaultPagination()
	if req.SortBy == "" {
		req.SortBy = "created_at"
	}
	if req.SortOrder == "" {
		req.SortOrder = "desc"
	}

	items, total, userCountMap, err := h.S.List(c.Request.Context(), svc.ListTenantsOption{
		Page:      req.Page,
		PageSize:  req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
		Keyword:   req.Q,
		Status:    req.Status,
		Plan:      req.Plan,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}

	// 直接返回模型 + 附加 user_count 视图字段（匿名 struct），不新增 DTO 包
	type tenantView struct {
		mdltenant.Tenant
		UserCount int64 `json:"user_count"`
	}
	out := make([]tenantView, 0, len(items))
	for i := range items {
		out = append(out, tenantView{
			Tenant:    items[i],
			UserCount: userCountMap[items[i].UUID.String()],
		})
	}

	dto.ResponseList(c, out, &dto.PaginationResponse{
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
}

// GET /api/v1/admin/tenants/:id
func (h *TenantHandler) GetTenant(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	res, err := h.S.Get(c.Request.Context(), id)
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "tenant not found", err)
		return
	}
	dto.ResponseSuccess(c, res) // 直接返回 iam.Tenant
}

// -------- Handlers：Create / Upsert --------

// POST /api/v1/admin/tenants
// Create：严格创建（存在即报错）
type CreateTenantReq struct {
	Key         string  `json:"key"          validate:"required,alphanumdash"`
	Name        string  `json:"name"         validate:"required"`
	Plan        string  `json:"plan"         validate:"required"`
	Domain      *string `json:"domain"`
	Description *string `json:"description"`
	Status      *int16  `json:"status"`

	// 可选：初始化管理员（仅在创建时生效）
	AdminUserName    *string `json:"admin_username"` // 必填之一：AdminUserName / AdminEmail / AdminPhone
	AdminPassword    *string `json:"admin_password"` // 若设管理员则必填
	AdminEmail       *string `json:"admin_email"`
	AdminPhone       *string `json:"admin_phone"`
	AdminDisplayName *string `json:"admin_display_name"`
	AdminAvatarURL   *string `json:"admin_avatar_url"`
	AssignOwnerRole  *bool   `json:"assign_owner_role"` // 默认 true：额外赋予 role_owner
}

// Upsert：幂等保存；如果是“首次创建”，可同时初始化管理员
type UpsertTenantReq struct {
	Key         string  `json:"key"          validate:"required,alphanumdash"`
	Name        string  `json:"name"         validate:"required"`
	Plan        string  `json:"plan"         validate:"required"`
	Domain      *string `json:"domain"`
	Description *string `json:"description"`
	Status      *int16  `json:"status"`

	// 可选：仅当 key 不存在 → 首次创建时才会执行初始化管理员
	AdminUserName    *string `json:"admin_username"`
	AdminPassword    *string `json:"admin_password"`
	AdminEmail       *string `json:"admin_email"`
	AdminPhone       *string `json:"admin_phone"`
	AdminDisplayName *string `json:"admin_display_name"`
	AdminAvatarURL   *string `json:"admin_avatar_url"`
	AssignOwnerRole  *bool   `json:"assign_owner_role"`
}

// ---------- Handlers：Create / Upsert ----------

func (h *TenantHandler) CreateTenant(c *gin.Context) {
	var req CreateTenantReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	id, err := h.S.Create(c.Request.Context(), svc.CreateTenantInput{
		Key:         req.Key,
		Name:        req.Name,
		Plan:        req.Plan,
		Domain:      req.Domain,
		Description: req.Description,
		Status:      req.Status,
		InitAdmin: &svc.InitAdminInput{
			UserName:    req.AdminUserName,
			Password:    req.AdminPassword,
			Email:       req.AdminEmail,
			Phone:       req.AdminPhone,
			DisplayName: req.AdminDisplayName,
			AvatarURL:   req.AdminAvatarURL,
			AssignOwner: req.AssignOwnerRole,
		},
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "创建失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"id": id})
}

func (h *TenantHandler) UpsertTenant(c *gin.Context) {
	var req UpsertTenantReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	id, err := h.S.Upsert(c.Request.Context(), svc.UpsertTenantInput{
		Key:         req.Key,
		Name:        req.Name,
		Plan:        req.Plan,
		Domain:      req.Domain,
		Description: req.Description,
		Status:      req.Status,
		InitAdmin: &svc.InitAdminInput{
			UserName:    req.AdminUserName,
			Password:    req.AdminPassword,
			Email:       req.AdminEmail,
			Phone:       req.AdminPhone,
			DisplayName: req.AdminDisplayName,
			AvatarURL:   req.AdminAvatarURL,
			AssignOwner: req.AssignOwnerRole,
		},
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "保存失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"id": id})
}

func (h *TenantHandler) DeleteTenant(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.S.Delete(c.Request.Context(), id); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "删除失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}
