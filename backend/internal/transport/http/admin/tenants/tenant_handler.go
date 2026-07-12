// api/http/admin/tenants/tenant_handler.go
package tenants

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	svc "github.com/ArtisanCloud/PowerX/internal/service/tenant"
	mdltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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
	Q          string  `form:"q"`           // 模糊 name/domain
	TenantUUID string  `form:"tenant_uuid"` // 精确 UUID 查询
	Status     *string `form:"status"`      // 可选：active/inactive/suspended...
	Plan       *string `form:"plan"`        // 可选：套餐过滤
	SortBy     string  `form:"sort_by"`     // 默认 created_at
	SortOrder  string  `form:"sort_order"`  // 默认 desc
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

	if tenantUUID := strings.TrimSpace(req.TenantUUID); tenantUUID != "" {
		if !reqctx.IsRoot(c.Request.Context()) {
			currentTenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
			if currentTenantUUID == "" || !strings.EqualFold(currentTenantUUID, tenantUUID) {
				dto.ResponseError(c, http.StatusForbidden, "tenant access denied", nil)
				return
			}
		}
		item, err := h.S.GetByUUID(c.Request.Context(), tenantUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				dto.ResponseError(c, http.StatusNotFound, "tenant not found", err)
				return
			}
			dto.ResponseValidationError(c, err)
			return
		}
		dto.ResponseList(c, tenantViews([]mdltenant.Tenant{*item}, map[string]int64{}), &dto.PaginationResponse{
			Total:    1,
			Page:     1,
			PageSize: 1,
		})
		return
	}

	if !reqctx.IsRoot(c.Request.Context()) {
		dto.ResponseError(c, http.StatusForbidden, "root permission required to list tenants", nil)
		return
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

	dto.ResponseList(c, tenantViews(items, userCountMap), &dto.PaginationResponse{
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

type tenantView struct {
	mdltenant.Tenant
	UserCount int64 `json:"user_count"`
}

func tenantViews(items []mdltenant.Tenant, userCountMap map[string]int64) []tenantView {
	out := make([]tenantView, 0, len(items))
	for i := range items {
		out = append(out, tenantView{
			Tenant:    items[i],
			UserCount: userCountMap[items[i].UUID.String()],
		})
	}
	return out
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
