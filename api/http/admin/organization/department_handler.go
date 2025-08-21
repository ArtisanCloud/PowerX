package organization

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	orgsvc "github.com/ArtisanCloud/PowerX/internal/service/organization"
	auth "github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

type DepartmentHandler struct {
	Svc      *orgsvc.OrgService
	DeptRepo *repoi.DepartmentRepository
}

func NewDepartmentHandler(s *orgsvc.OrgService, dr *repoi.DepartmentRepository) *DepartmentHandler {
	return &DepartmentHandler{Svc: s, DeptRepo: dr}
}

// POST /api/v1/admin/organization/departments
func (h *DepartmentHandler) Create(c *gin.Context) {
	var req CreateDepartmentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	tidStr := auth.GetTenantID(ctx)
	if tidStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id missing"})
		return
	}
	tid, err := strconv.ParseUint(tidStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	dept := &m.Department{
		TenantID:       tid,
		Name:           req.Name,
		Key:            req.Key,
		ParentID:       req.ParentID,
		LeaderMemberID: req.LeaderMemberID,
		Meta:           req.Meta,
	}
	if req.Sort != nil {
		dept.Sort = *req.Sort
	}
	if req.Status != nil {
		dept.Status = *req.Status
	} else {
		dept.Status = 1
	}

	if err := h.Svc.CreateDepartment(ctx, dept, req.ParentID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toDTO(dept))
}

// PATCH /api/v1/admin/organization/departments/:id
func (h *DepartmentHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var req UpdateDepartmentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	tidStr := auth.GetTenantID(ctx)
	if tidStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id missing"})
		return
	}
	tid, err := strconv.ParseUint(tidStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	err = h.Svc.UpdateDepartment(ctx, tid, id, orgsvc.UpdateDepartmentOpts{
		Name:           req.Name,
		Key:            req.Key,
		NewParentID:    req.NewParentID,
		Sort:           req.Sort,
		LeaderMemberID: req.LeaderMemberID,
		Status:         req.Status,
		Meta:           req.Meta,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dept, err := h.DeptRepo.FindByID(ctx, tid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "department not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toDTO(dept))
}

// DELETE /api/v1/admin/organization/departments/:id[?force=true]
func (h *DepartmentHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	ctx := c.Request.Context()
	tidStr := auth.GetTenantID(ctx)
	if tidStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id missing"})
		return
	}
	tid, err := strconv.ParseUint(tidStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	force := c.Query("force") == "true"
	if err := h.Svc.DeleteDepartment(ctx, tid, id, force); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /api/v1/admin/organization/departments/tree
func (h *DepartmentHandler) Tree(c *gin.Context) {
	ctx := c.Request.Context()
	tidStr := auth.GetTenantID(ctx)
	if tidStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id missing"})
		return
	}
	tid, err := strconv.ParseUint(tidStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
		return
	}

	nodes, err := h.Svc.GetDepartmentTree(ctx, tid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, nodes)
}

// DTO 映射：字段直接对应 GORM 模型
func toDTO(d *m.Department) *DepartmentDTO {
	return &DepartmentDTO{
		ID: d.ID, TenantID: d.TenantID, Name: d.Name, Key: d.Key,
		ParentID: d.ParentID, Path: d.Path, Depth: d.Depth,
		Sort: d.Sort, LeaderMemberID: d.LeaderMemberID, Status: d.Status,
		Meta: d.Meta,
	}
}
