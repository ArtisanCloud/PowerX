package iam

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/service"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"sort"
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
				return service.ErrInvalidKey
			}
			var count int64
			if err := tx.Model(&m.Department{}).
				Where("tenant_id=? AND key=?", d.TenantID, d.Key).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("%w: %s", service.ErrKeyExists, d.Key)
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
					return fmt.Errorf("%w: id=%d", service.ErrParentNotFound, *parentID)
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
	LeaderMemberID *uint64
	Status         *int16
	Meta           any
	HasNewParentID bool
}

// Update（含可选 Move）
func (s *OrgService) UpdateDepartment(ctx context.Context, tenantID, deptID uint64, opt UpdateDepartmentOpts) error {
	return s.TX(ctx, func(tx *gorm.DB) error {
		if opt.NewParentID != nil && *opt.NewParentID == 0 {
			opt.NewParentID = nil
		}

		var d m.Department
		if err := tx.Where("tenant_id=? AND id=?", tenantID, deptID).First(&d).Error; err != nil {
			return err
		}

		// 1) 先更新非层级字段
		patch := map[string]any{}
		if opt.Name != nil {
			patch["name"] = *opt.Name
		}
		if opt.Key != nil { // ← 处理 key
			k := strings.TrimSpace(*opt.Key)
			if !isValidKey(k) {
				return service.ErrInvalidKey // e.g. "only [a-z0-9-], max 64"
			}
			// 唯一性（同租户、排除自己）
			var cnt int64
			if err := tx.Model(&m.Department{}).
				Where("tenant_id=? AND key=? AND id<>?", tenantID, k, deptID).
				Count(&cnt).Error; err != nil {
				return err
			}
			if cnt > 0 {
				return fmt.Errorf("%w: %s", service.ErrKeyExists, k)
			}
			patch["key"] = k
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
		if opt.HasNewParentID {
			return s.moveDepartmentTx(ctx, tx, &d, opt.NewParentID)
		}
		return nil
	})
}

// newParentID == nil 表示把部门移动为根节点
func (s *OrgService) moveDepartmentTx(ctx context.Context, tx *gorm.DB, d *m.Department, newParentID *uint64) error {
	tDept := (&m.Department{}).GetTableName(true)

	// —— 兜底：把 0 视为根（外层也做过，这里再保险）——
	if newParentID != nil && *newParentID == 0 {
		newParentID = nil
	}

	// 1) 读取新父节点并做成环校验（仅当 newParentID != nil）
	var parent m.Department
	if newParentID != nil {
		if err := tx.Where("tenant_id=? AND id=?", d.TenantID, *newParentID).First(&parent).Error; err != nil {
			// 这里会被 handler 翻译成 “上级部门不存在”
			return err
		}
		// 禁止把节点挂到它自己的子树下（避免形成环）
		// d.Path 形如 "/.../<d.ID>/"；若 parent.Path 以 d.Path 为前缀，说明 parent 在 d 的子树中
		if strings.HasPrefix(parent.Path, d.Path) {
			return service.ErrMoveCreatesCycle
		}
	}

	// 2) 计算新前缀与新深度
	// oldPrefix 一定包含自身ID："/.../<d.ID>/"
	oldPrefix := d.Path
	var newPrefix string
	var newDepth int
	if newParentID == nil {
		// —— 移动为根 —— path = "/<d.ID>/", depth = 0
		newPrefix = fmt.Sprintf("/%d/", d.ID)
		newDepth = 0
	} else {
		// —— 移动到新父节点下 —— path = parent.Path + "<d.ID>/", depth = parent.Depth + 1
		newPrefix = parent.Path + fmt.Sprintf("%d/", d.ID)
		newDepth = parent.Depth + 1
	}
	deltaDepth := newDepth - d.Depth

	// 3) 批量更新这棵子树（包含自身）的 path/depth（一次 SQL 前缀替换 + 深度平移）
	if err := tx.Exec(`
		UPDATE `+tDept+` AS dd
		   SET path  = regexp_replace(dd.path, '^'||?, ?),
		       depth = dd.depth + ?
		 WHERE dd.tenant_id = ? AND dd.path LIKE ?`,
		strings.TrimRight(oldPrefix, "/"), // '^/old/.../<d.ID>'
		strings.TrimRight(newPrefix, "/"), // '/new/.../<d.ID>'
		deltaDepth,
		d.TenantID,
		oldPrefix+"%",
	).Error; err != nil {
		return err
	}

	// 4) 重建闭包表（同事务）：先删除子树所有闭包边，再按新的父子关系逐个补回
	if err := s.closRepo.RebuildSubtreeTx(ctx, tx, d.TenantID, d.ID); err != nil {
		return err
	}

	// 先查出子树（用“新前缀”），按 depth 升序，确保父先于子处理
	var subtree []struct {
		ID       uint64
		ParentID *uint64
		Depth    int
	}
	if err := tx.Raw(`
		SELECT id, parent_id, depth
		  FROM `+tDept+`
		 WHERE tenant_id=? AND path LIKE ?
		 ORDER BY depth ASC`,
		d.TenantID, newPrefix+"%",
	).Scan(&subtree).Error; err != nil {
		return err
	}

	for _, n := range subtree {
		// self 边
		if err := s.closRepo.EnsureSelfEdgeTx(ctx, tx, d.TenantID, n.ID); err != nil {
			return err
		}
		// 决定这个节点的“当前父亲”：
		//   - 根节点 d：父亲 = newParentID（此时若移动为根则为 nil，不继承任何祖先）
		//   - 其它节点：父亲仍是它自己的 ParentID（没变）
		var parentForThis *uint64
		if n.ID == d.ID {
			parentForThis = newParentID
		} else {
			parentForThis = n.ParentID
		}
		if parentForThis != nil {
			// 从父亲的所有祖先继承（父亲必须已处理过 self/继承，因我们按 depth 升序）
			if err := s.closRepo.InheritFromParentTx(ctx, tx, d.TenantID, *parentForThis, n.ID); err != nil {
				return err
			}
		}
	}

	// 5) 更新当前节点的 parent_id 字段（根则置 NULL）
	if err := tx.Model(&m.Department{}).
		Where("tenant_id=? AND id=?", d.TenantID, d.ID).
		Update("parent_id", newParentID).Error; err != nil {
		return err
	}

	// 同步内存值（可选）
	d.ParentID = newParentID
	d.Path = newPrefix
	d.Depth = newDepth

	return nil
}

// Delete
func (s *OrgService) DeleteDepartment(ctx context.Context, tenantID, deptID uint64, force bool) error {
	return s.TX(ctx, func(tx *gorm.DB) error {
		tDept := (&m.Department{}).GetTableName(true)

		// 先取待删节点，拿到它的 path 前缀
		var d m.Department
		if err := tx.Where("tenant_id=? AND id=?", tenantID, deptID).First(&d).Error; err != nil {
			return err
		}

		// ✅ 正确的子节点计数：直接用 d.Path 前缀
		var cnt int64
		if err := tx.Raw(
			`SELECT COUNT(1) FROM `+tDept+` WHERE tenant_id=? AND path LIKE ? AND id <> ?`,
			tenantID, d.Path+"%", deptID,
		).Scan(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 && !force {
			return errors.New("department has children; use ?force=true to force delete")
		}

		// （可选）还有成员则不允许删
		// TODO: 检查 member_department / assignment 关系

		// 删除闭包边 + 删除部门
		if !force {
			// 仅删单节点
			if err := tx.Where("tenant_id=? AND (ancestor_id=? OR descendant_id=?)",
				tenantID, deptID, deptID).Delete(&m.DepartmentClosure{}).Error; err != nil {
				return err
			}
			return tx.Where("tenant_id=? AND id=?", tenantID, deptID).
				Delete(&m.Department{}).Error
		}

		// force=true：删整棵子树
		if err := s.closRepo.DeleteSubtreeTx(ctx, tx, tenantID, d.ID); err != nil {
			return err
		}
		return tx.Where("tenant_id=? AND path LIKE ?", tenantID, d.Path+"%").
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
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
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

func (s *OrgService) GetDepartmentTree(ctx context.Context, tenantID uint64) ([]*m.Department, error) {
	var ds []m.Department
	if err := s.DB.WithContext(ctx).
		Where("tenant_id=?", tenantID).
		Order("path ASC"). // 父在前、子在后
		Find(&ds).Error; err != nil {
		return nil, err
	}

	byID := make(map[uint64]*m.Department, len(ds))
	var roots []*m.Department

	// 先建所有节点（只拷你需要的字段）
	for i := range ds {
		d := &ds[i]
		n := &m.Department{
			PowerModel:     model.PowerModel{ID: d.ID},
			Name:           d.Name,
			Sort:           d.Sort,
			LeaderMemberID: d.LeaderMemberID,
			ParentID:       d.ParentID,
			Key:            d.Key,
			Status:         d.Status,
			Meta:           d.Meta,
		}
		byID[n.ID] = n
		if n.ParentID == nil {
			roots = append(roots, n)
		}
	}

	// 挂子节点
	for i := range ds {
		if ds[i].ParentID != nil {
			if p := byID[*ds[i].ParentID]; p != nil {
				p.Children = append(p.Children, byID[ds[i].ID])
			}
		}
	}

	// 排序器：先按 sort 升序，再按 id 升序
	less := func(a, b *m.Department) bool {
		if a.Sort != b.Sort {
			return a.Sort < b.Sort
		}
		return a.ID < b.ID
	}

	// 递归对每个父节点的 Children 排序
	var sortRec func(n *m.Department)
	sortRec = func(n *m.Department) {
		if len(n.Children) == 0 {
			return
		}
		sort.Slice(n.Children, func(i, j int) bool { return less(n.Children[i], n.Children[j]) })
		for _, ch := range n.Children {
			sortRec(ch)
		}
	}

	// 先排根，再递归排每棵子树
	sort.Slice(roots, func(i, j int) bool { return less(roots[i], roots[j]) })
	for _, r := range roots {
		sortRec(r)
	}

	return roots, nil
}
