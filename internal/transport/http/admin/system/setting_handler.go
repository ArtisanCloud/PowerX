package system

import (
	"encoding/json"
	"gorm.io/datatypes"
	"net/http"
	"strconv"
	"strings"

	settingsvc "github.com/ArtisanCloud/PowerX/internal/service/system"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
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
	TenantID *uint64 `form:"tenant_id"`
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
	tenantID, _ := strconv.ParseUint(c.Param("tenant_id"), 10, 64)
	var req ListTenantSettingsReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()
	items, total, err := h.S.ListTenant(c.Request.Context(), tenantID, strings.TrimSpace(req.Prefix), req.Page, req.PageSize)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询租户设置失败", err)
		return
	}
	dto.ResponseList(c, items, &dto.PaginationResponse{Total: total, Page: req.Page, PageSize: req.PageSize})
}

func (h *SettingHandler) GetTenant(c *gin.Context) {
	tenantID, _ := strconv.ParseUint(c.Param("tenant_id"), 10, 64)
	key := strings.TrimSpace(c.Param("key"))
	item, err := h.S.GetTenant(c.Request.Context(), tenantID, key)
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "未找到租户设置", err)
		return
	}
	dto.ResponseSuccess(c, item)
}

func (h *SettingHandler) UpsertTenant(c *gin.Context) {
	tenantID, _ := strconv.ParseUint(c.Param("tenant_id"), 10, 64)
	key := strings.TrimSpace(c.Param("key"))
	var req UpsertSystemSettingReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if err := h.S.UpsertTenant(c.Request.Context(), tenantID, key, req.ValueJSON, req.Group, req.Description, req.Editable); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "写入租户设置失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *SettingHandler) DeleteTenant(c *gin.Context) {
	tenantID, _ := strconv.ParseUint(c.Param("tenant_id"), 10, 64)
	key := strings.TrimSpace(c.Param("key"))
	var req UpsertSystemSettingReq
	_ = c.ShouldBindJSON(&req)
	soft := true
	if req.Soft != nil {
		soft = *req.Soft
	}
	if err := h.S.DeleteTenant(c.Request.Context(), tenantID, key, soft); err != nil {
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
	val, source, err := h.S.GetEffectiveFromDB(c.Request.Context(), req.TenantID, key)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "读取生效值失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"source": source, "value_json": json.RawMessage(val)})
}
