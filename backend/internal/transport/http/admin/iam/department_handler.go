package iam

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	orgsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DepartmentHandler struct {
	Svc      *orgsvc.OrgService
	DeptRepo *repoi.DepartmentRepository
}

func NewDepartmentHandler(deps *shared.Deps) *DepartmentHandler {
	return &DepartmentHandler{
		Svc:      orgsvc.NewOrgService(deps.DB),
		DeptRepo: repoi.NewDepartmentRepository(deps.DB),
	}
}

func (h *DepartmentHandler) tenantUUIDFromContext(c *gin.Context) (string, bool) {
	return requireTenantUUIDFromContext(c)
}

// POST /api/v1/admin/organization/departments
func (h *DepartmentHandler) Create(c *gin.Context) {
	var req CreateDepartmentReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	tenantUUID, ok := h.tenantUUIDFromContext(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// 可选兼容：把 0 视为根（不需要可去掉）
	if req.ParentID != nil && *req.ParentID == 0 {
		req.ParentID = nil
	}

	dept := &m.Department{
		TenantUUID:     tenantUUID,
		Name:           req.Name,
		Key:            "", // 若允许后端生成 key，这里留空；前端传了就用
		ParentID:       req.ParentID,
		LeaderMemberID: req.LeaderMemberID,
		Meta:           req.Meta,
		Status:         1, // 默认启用
	}
	if req.Key != nil {
		dept.Key = *req.Key
	}
	if req.Sort != nil {
		dept.Sort = *req.Sort
	}
	if req.Status != nil {
		dept.Status = *req.Status
	}

	if err := h.Svc.CreateDepartment(ctx, dept, req.ParentID); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "创建部门失败", err)
		return
	}
	dto.ResponseSuccess(c, toDTO(dept, tenantUUID))
}

// PATCH /api/v1/admin/organization/departments/:id
func (h *DepartmentHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var req UpdateDepartmentReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	hasNewParent := req.NewParentID != nil
	// 0 -> 根
	if hasNewParent && *req.NewParentID == 0 {
		req.NewParentID = nil
	}

	tenantUUID, ok := h.tenantUUIDFromContext(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	err := h.Svc.UpdateDepartment(ctx, tenantUUID, id, orgsvc.UpdateDepartmentOpts{
		Name:           req.Name,
		Key:            req.Key,
		NewParentID:    req.NewParentID,
		Sort:           req.Sort,
		LeaderMemberID: req.LeaderMemberID,
		Status:         req.Status,
		Meta:           req.Meta,
		HasNewParentID: hasNewParent,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新部门失败", err)
		return
	}

	dept, err := h.DeptRepo.FindByID(ctx, tenantUUID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dto.ResponseError(c, http.StatusNotFound, "department not found", nil)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "查询部门失败", err)
		return
	}
	dto.ResponseSuccess(c, toDTO(dept, tenantUUID))
}

// DELETE /api/v1/admin/organization/departments/:id[?force=true]
func (h *DepartmentHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	tenantUUID, ok := h.tenantUUIDFromContext(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	force := c.Query("force") == "true"
	if err := h.Svc.DeleteDepartment(ctx, tenantUUID, id, force); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "删除部门失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// GET /api/v1/admin/organization/departments/tree
func (h *DepartmentHandler) Tree(c *gin.Context) {
	tenantUUID, ok := h.tenantUUIDFromContext(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	nodes, err := h.Svc.GetDepartmentTree(ctx, tenantUUID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询部门树失败", err)
		return
	}
	if nodes == nil {
		nodes = []*m.Department{}
	}
	dto.ResponseSuccess(c, nodes)
}

// DTO 映射
func toDTO(d *m.Department, tenantUUID string) *DepartmentDTO {
	return &DepartmentDTO{
		ID: d.ID, TenantUUID: tenantUUID,
		Name: d.Name, Key: d.Key,
		ParentID: d.ParentID, Path: d.Path, Depth: d.Depth,
		Sort: d.Sort, LeaderMemberID: d.LeaderMemberID, Status: d.Status,
		Meta: d.Meta,
	}
}
