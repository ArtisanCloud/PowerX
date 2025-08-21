package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

type OrgService struct {
	db       *gorm.DB
	deptRepo *repoi.DepartmentRepository
	closRepo *repoi.DepartmentClosureRepository
}

func NewOrgService(db *gorm.DB) *OrgService {
	return &OrgService{
		db:       db,
		deptRepo: repoi.NewDepartmentRepository(db),
		closRepo: repoi.NewDepartmentClosureRepository(db),
	}
}

func (s *OrgService) tx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// Create
func (s *OrgService) CreateDepartment(ctx context.Context, d *m.Department, parentID *uint64) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(d).Error; err != nil {
			return err
		}

		var path string
		var depth int
		if parentID != nil {
			var p m.Department
			if err := tx.Where("tenant_id=? AND id=?", d.TenantID, *parentID).First(&p).Error; err != nil {
				return err
			}
			path = p.Path + fmt.Sprintf("%d/", d.ID)
			depth = p.Depth + 1
		} else {
			path = fmt.Sprintf("/%d/", d.ID)
			depth = 0
		}
		if err := tx.Model(d).Updates(map[string]any{"path": path, "depth": depth}).Error; err != nil {
			return err
		}

		// 闭包表：self + 继承父系
		if err := s.closRepo.EnsureSelfEdge(ctx, d.TenantID, d.ID); err != nil {
			return err
		}
		if parentID != nil {
			if err := s.closRepo.InheritFromParent(ctx, d.TenantID, *parentID, d.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

type UpdateDepartmentOpts struct {
	Name           *string
	Key            *string
	NewParentID    *uint64
	Sort           *int
	Code           *string
	LeaderMemberID *uint64
	Status         *int16
	Meta           any
}

// Update（含可选 Move）
func (s *OrgService) UpdateDepartment(ctx context.Context, tenantID, deptID uint64, opt UpdateDepartmentOpts) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		var d m.Department
		if err := tx.Where("tenant_id=? AND id=?", tenantID, deptID).First(&d).Error; err != nil {
			return err
		}

		// 1) 先更新非层级字段
		patch := map[string]any{}
		if opt.Name != nil {
			patch["name"] = *opt.Name
		}
		if opt.Code != nil {
			patch["code"] = *opt.Code
		}
		if opt.Sort != nil {
			patch["sort"] = *opt.Sort
		}
		if opt.LeaderMemberID != nil {
			patch["leader_member_id"] = *opt.LeaderMemberID
		}
		if opt.Status != nil {
			patch["status"] = *opt.Status
		}
		if opt.Meta != nil {
			patch["meta"] = opt.Meta
		}
		if len(patch) > 0 {
			if err := tx.Model(&d).Updates(patch).Error; err != nil {
				return err
			}
		}

		// 2) 若需要移动父节点
		if opt.NewParentID != nil {
			return s.moveDepartmentTx(ctx, tx, &d, *opt.NewParentID)
		}
		return nil
	})
}

func (s *OrgService) moveDepartmentTx(ctx context.Context, tx *gorm.DB, d *m.Department, newParentID uint64) error {
	if d.ParentID != nil && *d.ParentID == newParentID {
		return nil
	}
	var p m.Department
	if err := tx.Where("tenant_id=? AND id=?", d.TenantID, newParentID).First(&p).Error; err != nil {
		return err
	}

	oldPrefix := d.Path
	newPrefix := p.Path + fmt.Sprintf("%d/", d.ID)
	deltaDepth := int(p.Depth + 1 - d.Depth)

	// 1) 批量更新子树 path/depth
	tDept := d.GetTableName(true)
	if err := tx.Exec(`
		UPDATE `+tDept+` AS dd
		   SET path  = regexp_replace(dd.path, '^'||?, ?),
		       depth = dd.depth + ?
		 WHERE dd.tenant_id = ? AND dd.path LIKE ?`,
		strings.TrimRight(oldPrefix, "/"),
		strings.TrimRight(newPrefix, "/"),
		deltaDepth, d.TenantID, oldPrefix+"%",
	).Error; err != nil {
		return err
	}

	// 2) 重建子树闭包边
	if err := s.closRepo.RebuildSubtree(ctx, d.TenantID, d.ID); err != nil {
		return err
	}

	// 3) 重新插入子树 self & 继承新父系
	//   取新前缀下所有节点
	var descendants []struct{ ID uint64 }
	if err := tx.Raw(`SELECT id FROM `+tDept+` WHERE tenant_id=? AND path LIKE ?`,
		d.TenantID, newPrefix+"%",
	).Scan(&descendants).Error; err != nil {
		return err
	}

	for _, row := range descendants {
		if err := s.closRepo.EnsureSelfEdge(ctx, d.TenantID, row.ID); err != nil {
			return err
		}
		if err := s.closRepo.InheritFromParent(ctx, d.TenantID, newParentID, row.ID); err != nil {
			return err
		}
	}

	// 4) 更新当前节点父ID（如果你的 Department 有 ParentID 字段）
	if err := tx.Model(&m.Department{}).
		Where("tenant_id=? AND id=?", d.TenantID, d.ID).
		Update("parent_id", newParentID).Error; err != nil {
		return err
	}

	return nil
}

// Delete
func (s *OrgService) DeleteDepartment(ctx context.Context, tenantID, deptID uint64, force bool) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		// 不允许删有子节点的部门（除非 force）
		var cnt int64
		tDept := (&m.Department{}).GetTableName(true)
		if err := tx.Raw(`
		  SELECT COUNT(1) FROM `+tDept+`
		   WHERE tenant_id=? AND path LIKE (
		      SELECT CONCAT(path, id::text, '/') FROM `+tDept+` WHERE tenant_id=? AND id=?
		   ) || '%' AND id <> ?`,
			tenantID, tenantID, deptID, deptID,
		).Scan(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 && !force {
			return errors.New("department has children; use ?force=true to force delete")
		}

		// TODO: 校验是否还有成员在此部门（你可查 MemberDepartment/Assignment）

		// 先删闭包，再删部门
		if err := tx.Where("tenant_id=? AND (ancestor_id=? OR descendant_id=?)", tenantID, deptID, deptID).
			Delete(&m.DepartmentClosure{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id=? AND id=?", tenantID, deptID).
			Delete(&m.Department{}).Error
	})
}

// Tree（简单返回拍平的树节点，前端自行构树或这里直接构成嵌套）
type TreeNode struct {
	ID       uint64      `json:"id"`
	Name     string      `json:"name"`
	ParentID *uint64     `json:"parent_id,omitempty"`
	Children []*TreeNode `json:"children,omitempty"`
}

func (s *OrgService) GetDepartmentTree(ctx context.Context, tenantID uint64) ([]*TreeNode, error) {
	var ds []m.Department
	if err := s.db.WithContext(ctx).
		Where("tenant_id=?", tenantID).
		Order("path ASC"). // path 顺序天然是树的先序
		Find(&ds).Error; err != nil {
		return nil, err
	}
	byID := map[uint64]*TreeNode{}
	var roots []*TreeNode
	for i := range ds {
		n := &TreeNode{ID: ds[i].ID, Name: ds[i].Name, ParentID: ds[i].ParentID}
		byID[n.ID] = n
		if n.ParentID == nil {
			roots = append(roots, n)
		}
	}
	for i := range ds {
		if ds[i].ParentID != nil {
			p := byID[*ds[i].ParentID]
			if p != nil {
				p.Children = append(p.Children, byID[ds[i].ID])
			}
		}
	}
	return roots, nil
}
