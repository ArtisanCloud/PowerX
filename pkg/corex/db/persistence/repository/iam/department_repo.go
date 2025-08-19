package iam

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type DepartmentRepository struct {
	*repository.BaseRepository[dbm.Department]
	db *gorm.DB
}

func NewDepartmentRepository(db *gorm.DB) *DepartmentRepository {
	return &DepartmentRepository{
		BaseRepository: repository.NewBaseRepository[dbm.Department](db),
		db:             db,
	}
}

func (r *DepartmentRepository) FindByID(ctx context.Context, id uint64) (*dbm.Department, error) {
	var d dbm.Department
	if err := r.db.WithContext(ctx).First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DepartmentRepository) FindByKey(ctx context.Context, tenantID uint64, key string) (*dbm.Department, error) {
	var d dbm.Department
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND key = ?", tenantID, key).
		First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DepartmentRepository) ListByTenant(ctx context.Context, tenantID uint64) ([]dbm.Department, error) {
	var list []dbm.Department
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("path ASC, sort ASC, id ASC").
		Find(&list).Error
	return list, err
}

func (r *DepartmentRepository) ListChildren(ctx context.Context, tenantID, parentID uint64) ([]dbm.Department, error) {
	var list []dbm.Department
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND parent_id = ?", tenantID, parentID).
		Order("sort ASC, id ASC").
		Find(&list).Error
	return list, err
}

// CreateWithPath 创建部门并计算 path/depth
func (r *DepartmentRepository) CreateWithPath(ctx context.Context, d *dbm.Department) error {
	// 计算 path/depth
	if d.ParentID == nil {
		d.Path = "/"
		d.Depth = 0
	} else {
		p, err := r.FindByID(ctx, *d.ParentID)
		if err != nil {
			return err
		}
		d.Path = fmt.Sprintf("%s%d/", p.Path, p.ID)
		d.Depth = p.Depth + 1
	}
	return r.db.WithContext(ctx).Create(d).Error
}

// Move 移动部门到新的父节点（同时更新子孙节点 path/depth）
func (r *DepartmentRepository) Move(ctx context.Context, deptID uint64, newParentID *uint64) error {
	// 取目标部门与新父节点
	d, err := r.FindByID(ctx, deptID)
	if err != nil {
		return err
	}
	var newPath string
	var newDepth int
	if newParentID == nil {
		newPath, newDepth = "/", 0
	} else {
		p, err := r.FindByID(ctx, *newParentID)
		if err != nil {
			return err
		}
		newPath, newDepth = fmt.Sprintf("%s%d/", p.Path, p.ID), p.Depth+1
	}

	oldPrefix := fmt.Sprintf("%s%d/", d.Path, d.ID) // 旧子树前缀
	depthDelta := newDepth - d.Depth

	// 事务：更新自己 + 更新子孙
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 更新自身
		if err := tx.Model(d).
			Updates(map[string]any{"parent_id": newParentID, "path": newPath, "depth": newDepth}).Error; err != nil {
			return err
		}
		// 更新子孙：path 前缀替换 + depth 调整
		var children []dbm.Department
		if err := tx.Where("path LIKE ?", oldPrefix+"%").Find(&children).Error; err != nil {
			return err
		}
		for i := range children {
			ch := &children[i]
			ch.Path = strings.Replace(ch.Path, oldPrefix, fmt.Sprintf("%s%d/", newPath, d.ID), 1)
			ch.Depth = ch.Depth + depthDelta
			if err := tx.Model(ch).Updates(map[string]any{"path": ch.Path, "depth": ch.Depth}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
