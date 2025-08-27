package iam

import (
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type PermissionHandler struct{ svc *iamsvc.PermissionService }

func NewPermissionHandler(deps *bootstrap.Deps) *PermissionHandler {
	return &PermissionHandler{svc: iamsvc.NewPermissionService(deps.DB)}
}

type registerReq struct {
	Items []dbm.Permission `json:"items" binding:"required,min=1"`
}

func (h *PermissionHandler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	if err := h.svc.RegisterPermissions(c.Request.Context(), req.Items); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "注册权限失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"registered": len(req.Items)})
}

type PermissionListQuery struct {
	Plugin   string `form:"plugin"`
	Resource string `form:"resource"`
	Module   string `form:"module"`
	Type     string `form:"type"`
	Keyword  string `form:"keyword"`
	Status   string `form:"status"`
	Page     int    `form:"page,default=1"`
	Size     int    `form:"size,default=50"`
	Sort     string `form:"sort"` // "plugin asc, resource asc"
}

func (h *PermissionHandler) List(c *gin.Context) {
	var q PermissionListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}

	filter := map[string]string{
		"plugin":   strings.TrimSpace(q.Plugin),
		"resource": strings.TrimSpace(q.Resource),
		"module":   strings.TrimSpace(q.Module),
		"type":     strings.TrimSpace(q.Type),
		"status":   strings.TrimSpace(q.Status),
		"keyword":  strings.TrimSpace(q.Keyword),
	}

	rows, total, err := h.svc.ListPermissions(c.Request.Context(), filter, q.Page, q.Size, q.Sort)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "查询失败", err)
		return
	}

	dto.ResponseList(c, rows, &dto.PaginationResponse{
		Total: total, Page: q.Page, PageSize: q.Size,
	})
}

func (h *PermissionHandler) Catalog(c *gin.Context) {
	data, err := h.svc.ListCatalog(c.Request.Context())
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "查询失败", err)
		return
	}
	dto.ResponseSuccess(c, data)
}

/* ---------- 同步/差异 ---------- */

type syncReq struct {
	Source     string           `json:"source" binding:"required"`
	Introduced string           `json:"introduced"` // 可选
	DryRun     bool             `json:"dry_run"`
	Items      []dbm.Permission `json:"items" binding:"required,min=1"`
}

func (h *PermissionHandler) Sync(c *gin.Context) {
	var req syncReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	res, err := h.svc.SyncPermissions(c.Request.Context(), req.Source, req.Introduced, req.Items, req.DryRun)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "同步失败", err)
		return
	}
	dto.ResponseSuccess(c, res)
}

type setStatusReq struct {
	Status string `json:"status" binding:"required"` // active | deprecated
}

type updateReq struct {
	Description *string `json:"description"` // 仅允许改描述
	Status      *string `json:"status"`      // 可选：同时允许改状态
}

func (h *PermissionHandler) SetStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的ID", err)
		return
	}
	var req setStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	st := strings.ToLower(strings.TrimSpace(req.Status))
	if st != "active" && st != "deprecated" {
		dto.ResponseError(c, http.StatusBadRequest, "status 仅支持 active/ deprecated", nil)
		return
	}
	// 废弃时写 deprecated_at；恢复时清空
	var deprecatedAt *int64
	if st == "deprecated" {
		now := time.Now().Unix()
		deprecatedAt = &now
	}
	if err := h.svc.UpdatePermissionFields(c.Request.Context(), id, nil, &st, deprecatedAt); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新状态失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"id": id, "status": st})
}

func (h *PermissionHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的ID", err)
		return
	}
	var req updateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数绑定失败", err)
		return
	}
	var stPtr *string
	var depAt *int64
	if req.Status != nil {
		st := strings.ToLower(strings.TrimSpace(*req.Status))
		if st != "active" && st != "deprecated" {
			dto.ResponseError(c, http.StatusBadRequest, "status 仅支持 active/ deprecated", nil)
			return
		}
		stPtr = &st
		if st == "deprecated" {
			now := time.Now().Unix()
			depAt = &now
		} else {
			depAt = nil
		}
	}
	if err := h.svc.UpdatePermissionFields(c.Request.Context(), id, req.Description, stPtr, depAt); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"id": id})
}
