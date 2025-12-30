package system

import (
	"encoding/json"
	"net/http"
	"strings"

	settingsvc "github.com/ArtisanCloud/PowerX/internal/service/system"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type SettingHandler struct {
	S *settingsvc.SettingService
}

func NewSettingHandler(db *gorm.DB) *SettingHandler {
	return &SettingHandler{S: settingsvc.NewSettingService(db)}
}

// ===== DTOs =====
type ListSystemSettingsReq struct {
	Group  string `form:"group"`
	Prefix string `form:"prefix"`
	dto.PaginationRequest
}

type UpsertSystemSettingReq struct {
	Group       *string        `json:"group,omitempty"`
	Description *string        `json:"description,omitempty"`
	Editable    *bool          `json:"editable,omitempty"`
	ValueJSON   datatypes.JSON `json:"value_json" validate:"required"`
	Soft        *bool          `json:"soft,omitempty"` // 删除用
}

type ListTenantSettingsReq struct {
	Prefix string `form:"prefix"`
	dto.PaginationRequest
}

type GetEffectiveReq struct {
	TenantUUID *string `form:"tenant_uuid"`
}

// ===== 系统级 KV =====
func (h *SettingHandler) ListSystem(c *gin.Context) {
	var req ListSystemSettingsReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	items, total, err := h.S.ListSystem(c.Request.Context(), strings.TrimSpace(req.Group), strings.TrimSpace(req.Prefix), req.Page, req.PageSize)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询系统设置失败", err)
		return
	}
	dto.ResponseList(c, items, &dto.PaginationResponse{Total: total, Page: req.Page, PageSize: req.PageSize})
}

func (h *SettingHandler) GetSystem(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	item, err := h.S.GetSystem(c.Request.Context(), key)
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "未找到系统设置", err)
		return
	}
	dto.ResponseSuccess(c, item)
}

func (h *SettingHandler) UpsertSystem(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	var req UpsertSystemSettingReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if err := h.S.UpsertSystem(c.Request.Context(), key, req.ValueJSON, req.Group, req.Description, req.Editable); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "写入系统设置失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *SettingHandler) DeleteSystem(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	var req UpsertSystemSettingReq
	_ = c.ShouldBindJSON(&req) // 允许空 body
	soft := true
	if req.Soft != nil {
		soft = *req.Soft
	}
	if err := h.S.DeleteSystem(c.Request.Context(), key, soft); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "删除系统设置失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// ===== 租户级 KV =====
func (h *SettingHandler) ListTenant(c *gin.Context) {
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	var req ListTenantSettingsReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	items, total, err := h.S.ListTenant(c.Request.Context(), tenantUUID, strings.TrimSpace(req.Prefix), req.Page, req.PageSize)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询租户设置失败", err)
		return
	}
	dto.ResponseList(c, items, &dto.PaginationResponse{Total: total, Page: req.Page, PageSize: req.PageSize})
}

func (h *SettingHandler) GetTenant(c *gin.Context) {
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	key := strings.TrimSpace(c.Param("key"))
	item, err := h.S.GetTenant(c.Request.Context(), tenantUUID, key)
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "未找到租户设置", err)
		return
	}
	dto.ResponseSuccess(c, item)
}

func (h *SettingHandler) UpsertTenant(c *gin.Context) {
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	key := strings.TrimSpace(c.Param("key"))
	var req UpsertSystemSettingReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if err := h.S.UpsertTenant(c.Request.Context(), tenantUUID, key, req.ValueJSON, req.Group, req.Description, req.Editable); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "写入租户设置失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *SettingHandler) DeleteTenant(c *gin.Context) {
	tenantUUID, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	key := strings.TrimSpace(c.Param("key"))
	var req UpsertSystemSettingReq
	_ = c.ShouldBindJSON(&req)
	soft := true
	if req.Soft != nil {
		soft = *req.Soft
	}
	if err := h.S.DeleteTenant(c.Request.Context(), tenantUUID, key, soft); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "删除租户设置失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// ===== 生效值（仅 DB 层合并：tenant > system） =====
func (h *SettingHandler) GetEffective(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	var req GetEffectiveReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	var tenantUUID *string
	rawTenant := ""
	if req.TenantUUID != nil {
		rawTenant = strings.TrimSpace(*req.TenantUUID)
	}
	if rawTenant == "" {
		rawTenant = strings.TrimSpace(reqctx.TenantUUIDFromGin(c))
	}
	if rawTenant != "" {
		canonical, err := reqctx.CanonicalTenantUUID(rawTenant)
		if err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid 格式错误", err)
			return
		}
		tenantUUID = &canonical
	}
	val, source, err := h.S.GetEffectiveFromDB(c.Request.Context(), tenantUUID, key)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "读取生效值失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"source": source, "value_json": json.RawMessage(val)})
}

func requireTenantUUIDFromContext(c *gin.Context) (string, bool) {
	tenantUUID, err := reqctx.RequireTenantUUIDValueFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return "", false
	}
	return tenantUUID.String(), true
}
