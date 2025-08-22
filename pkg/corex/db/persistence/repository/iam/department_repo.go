package iam

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
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

// ✅ 必须带 tenantID
func (r *DepartmentRepository) FindByID(ctx context.Context, tenantID, id uint64) (*dbm.Department, error) {
	var d dbm.Department
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&d).Error; err != nil {
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

// ⚠️ 如仍保留该方法：必须先 Create 拿到 d.ID，path 里拼“自身 ID”
func (r *DepartmentRepository) CreateWithPath(ctx context.Context, d *dbm.Department) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先插入获取自增 ID
		if err := tx.Create(d).Error; err != nil {
			return err
		}
		var path string
		var depth int
		if d.ParentID != nil {
			// 父节点也要带 tenant 过滤
			p, err := r.FindByID(ctx, d.TenantID, *d.ParentID)
			if err != nil {
				return err
			}
			// path = 父.path + 自身ID + "/"
			path = fmt.Sprintf("%s%d/", p.Path, d.ID)
			depth = p.Depth + 1
		} else {
			// 根："/自身ID/"
			path = fmt.Sprintf("/%d/", d.ID)
			depth = 0
		}
		return tx.Model(d).Updates(map[string]any{"path": path, "depth": depth}).Error
	})
}

// 移动部门到新父节点（同一事务内批量更新子树 path/depth）
func (r *DepartmentRepository) Move(ctx context.Context, tenantID, deptID uint64, newParentID *uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 取待移动节点
		d, err := r.FindByID(ctx, tenantID, deptID)
		if err != nil {
			return err
		}

		// 计算新前缀（注意用“自身 ID”）
		var newPrefix string
		var newDepth int
		if newParentID != nil {
			p, err := r.FindByID(ctx, tenantID, *newParentID)
			if err != nil {
				return err
			}
			newPrefix = fmt.Sprintf("%s%d/", p.Path, d.ID)
			newDepth = p.Depth + 1
		} else {
			newPrefix = fmt.Sprintf("/%d/", d.ID)
			newDepth = 0
		}

		oldPrefix := d.Path // d.Path 已包含自身ID，直接当子树前缀
		delta := newDepth - d.Depth
		t := d.GetTableName(true)

		// 批量更新整棵子树（包含自身）
		if err := tx.Exec(`
			UPDATE `+t+` 
			   SET path  = regexp_replace(path, '^'||?, ?),
			       depth = depth + ?
			 WHERE tenant_id = ? AND path LIKE ?`,
			strings.TrimRight(oldPrefix, "/"),
			strings.TrimRight(newPrefix, "/"),
			delta,
			tenantID,
			oldPrefix+"%",
		).Error; err != nil {
			return err
		}

		// 单独更新 parent_id
		return tx.Model(&dbm.Department{}).
			Where("tenant_id = ? AND id = ?", tenantID, d.ID).
			Update("parent_id", newParentID).Error
	})
}
