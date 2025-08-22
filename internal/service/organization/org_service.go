package organization

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/service"
	"strings"

	"gorm.io/gorm"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repoi "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

type OrgService struct {
	*service.BaseService

	deptRepo *repoi.DepartmentRepository
	closRepo *repoi.DepartmentClosureRepository
}

func NewOrgService(db *gorm.DB) *OrgService {
	return &OrgService{
		BaseService: &service.BaseService{
			DB: db,
		},
		deptRepo: repoi.NewDepartmentRepository(db),
		closRepo: repoi.NewDepartmentClosureRepository(db),
	}
}

// Create
// internal/service/organization/org_service.go
func (s *OrgService) CreateDepartment(ctx context.Context, d *m.Department, parentID *uint64) error {
	return s.TX(ctx, func(tx *gorm.DB) error {
		// 可选：0 当作根（前端误传 0 时兜底）
		if parentID != nil && *parentID == 0 {
			parentID = nil
		}

		// ① key 生成或校验
		if strings.TrimSpace(d.Key) == "" {
			k, err := s.generateDeptKey(ctx, tx, d.TenantID, d.Name)
			if err != nil {
				return err
			}
			d.Key = k
		} else {
			if !isValidKey(d.Key) {
				return ErrInvalidKey
			}
			var count int64
			if err := tx.Model(&m.Department{}).
				Where("tenant_id=? AND key=?", d.TenantID, d.Key).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("%w: %s", ErrKeyExists, d.Key)
			}
		}

		// ② 插入拿到 ID
		if err := tx.Create(d).Error; err != nil {
			return err // 这里返回任何错误 => 整个事务回滚（包括上面的 Create）
		}

		// ③ 计算 path/depth
		var path string
		var depth int
		if parentID != nil {
			var p m.Department
			if err := tx.Where("tenant_id=? AND id=?", d.TenantID, *parentID).First(&p).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("%w: id=%d", ErrParentNotFound, *parentID)
				}
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

		// ④ 闭包表（任一步出错同样回滚）
		if err := s.closRepo.EnsureSelfEdgeTx(ctx, tx, d.TenantID, d.ID); err != nil {
			return err
		}
		if parentID != nil {
			if err := s.closRepo.InheritFromParentTx(ctx, tx, d.TenantID, *parentID, d.ID); err != nil {
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
	return s.TX(ctx, func(tx *gorm.DB) error {
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
			return s.moveDepartmentTx(ctx, tx, &d, opt.NewParentID)
		}
		return nil
	})
}

func (s *OrgService) moveDepartmentTx(ctx context.Context, tx *gorm.DB, d *m.Department, newParentID *uint64) error {
	tDept := (&m.Department{}).GetTableName(true)

	// 1) 取新父节点（如果有），并做“不能把节点挂到自己的子树下”的校验
	var p m.Department
	if newParentID != nil {
		if err := tx.Where("tenant_id=? AND id=?", d.TenantID, *newParentID).First(&p).Error; err != nil {
			return err // handler 会把 ErrRecordNotFound 翻译成“上级部门不存在”
		}
		// p 在 d 的子树里？禁止，避免形成环
		// d.Path 形如 "/.../<d.ID>/"
		if strings.HasPrefix(p.Path, d.Path) {
			return ErrMoveCreatesCycle
		}
	}

	// oldPrefix 形如 "/.../<d.ID>/"
	oldPrefix := d.Path
	// newPrefix：根 or 新父路径 + 本节点 ID
	var newPrefix string
	var newDepth int
	if newParentID != nil {
		newPrefix = p.Path + fmt.Sprintf("%d/", d.ID)
		newDepth = p.Depth + 1
	} else {
		newPrefix = fmt.Sprintf("/%d/", d.ID)
		newDepth = 0
	}
	deltaDepth := newDepth - d.Depth

	// 2) 批量更新这棵子树的 path / depth（用前缀替换）
	if err := tx.Exec(`
		UPDATE `+tDept+` AS dd
		   SET path  = regexp_replace(dd.path, '^'||?, ?),
		       depth = dd.depth + ?
		 WHERE dd.tenant_id = ? AND dd.path LIKE ?`,
		strings.TrimRight(oldPrefix, "/"),
		strings.TrimRight(newPrefix, "/"),
		deltaDepth,
		d.TenantID,
		oldPrefix+"%",
	).Error; err != nil {
		return err
	}

	// 3) 先把子树里所有节点的闭包边删掉（Tx 版本，保证跟上面的 UPDATE 同事务）
	if err := s.closRepo.RebuildSubtreeTx(ctx, tx, d.TenantID, d.ID); err != nil {
		return err
	}

	// 4) 重新插入子树的闭包边：每个节点 = self 边 + 继承“它当前的 parent”
	//    注意：只有 d 的 parent 变成 newParentID；其它节点 parent 还是它们自己的 ParentID
	var subtree []struct {
		ID       uint64
		ParentID *uint64
		Depth    int
	}
	if err := tx.Raw(`
		SELECT id, parent_id, depth
		  FROM `+tDept+`
		 WHERE tenant_id=? AND path LIKE ?
		 ORDER BY depth ASC`, // 由浅入深，确保父亲先插边
		d.TenantID, newPrefix+"%",
	).Scan(&subtree).Error; err != nil {
		return err
	}

	for _, n := range subtree {
		// self 边
		if err := s.closRepo.EnsureSelfEdgeTx(ctx, tx, d.TenantID, n.ID); err != nil {
			return err
		}
		// 继承祖先：决定这个节点的“当前父亲”
		var parentForThis *uint64
		if n.ID == d.ID {
			parentForThis = newParentID // 根节点用新父
		} else {
			parentForThis = n.ParentID // 其它节点用原父
		}
		if parentForThis != nil {
			if err := s.closRepo.InheritFromParentTx(ctx, tx, d.TenantID, *parentForThis, n.ID); err != nil {
				return err
			}
		}
	}

	// 5) 更新当前节点的 ParentID 字段（根则置 NULL）
	if err := tx.Model(&m.Department{}).
		Where("tenant_id=? AND id=?", d.TenantID, d.ID).
		Update("parent_id", newParentID).Error; err != nil {
		return err
	}

	// 同步 d 的内存值（如果上层后续还用到）
	d.ParentID = newParentID
	d.Path = newPrefix
	d.Depth = newDepth

	return nil
}

// Delete
func (s *OrgService) DeleteDepartment(ctx context.Context, tenantID, deptID uint64, force bool) error {
	return s.TX(ctx, func(tx *gorm.DB) error {
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

func (s *OrgService) generateDeptKey(ctx context.Context, tx *gorm.DB, tenantID uint64, name string) (string, error) {
	base := slug(name) // "华东 销售部" -> "sales" 或 "huadong-xiaoshou-bu"（见下）
	if base == "" {
		base = "dept"
	}

	// 截断，预留后缀空间
	if len(base) > 48 {
		base = base[:48]
	}

	// 尝试 base、base-2、base-3… 直到唯一
	key := base
	for i := 1; ; i++ {
		var cnt int64
		if err := tx.Model(&m.Department{}).
			Where("tenant_id=? AND key=?", tenantID, key).
			Count(&cnt).Error; err != nil {
			return "", err
		}
		if cnt == 0 {
			return key, nil
		}
		// 追加数字后缀并确保不超过 64
		key = fmt.Sprintf("%s-%d", base, i)
		if len(key) > 64 {
			key = key[:64]
		}
		// 理论上很快就命中；极端情况下也会被唯一索引保护
	}
}

func isValidKey(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

// 极简 slug：小写、空白/下划线转连字符，移除其他不可用字符。
// 若你想中文转拼音，可后续换成拼音库；现在先走最小可用。
func slug(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b []rune
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b = append(b, r)
			lastDash = false
		case r == ' ' || r == '_' || r == '-':
			if !lastDash && len(b) > 0 {
				b = append(b, '-')
				lastDash = true
			}
		default:
			// 丢弃非 ASCII；中文场景会走到 "dept-2" 的后缀去重分支
		}
	}
	// 去掉首尾破折
	res := strings.Trim(string(b), "-")
	return res
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
	if err := s.DB.WithContext(ctx).
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
