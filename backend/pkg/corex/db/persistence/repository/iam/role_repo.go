package iam

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// RoleRepository：基础 CRUD + 常用查询
type RoleRepository struct {
	*repository.BaseRepository[dbm.Role]
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{
		BaseRepository: repository.NewBaseRepository[dbm.Role](db),
		db:             db,
	}
}

// ---------- 你已有 ----------

// FindByCode 在 scope+tenant 上按 code 查找
func (r *RoleRepository) FindByCode(ctx context.Context, scope string, tenantUUID *string, code string) (*dbm.Role, error) {
	var role dbm.Role
	q := r.db.WithContext(ctx).Where("scope = ? AND code = ?", scope, code)
	if scope == "tenant" && tenantUUID != nil {
		tn := strings.TrimSpace(*tenantUUID)
		if tn == "" {
			return nil, errors.New("tenant uuid required")
		}
		q = q.Where("tenant_uuid = ?", tn)
	} else {
		q = q.Where("(tenant_uuid = '' OR tenant_uuid IS NULL)")
	}
	if err := q.First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// EnsureDefaultRoles 仅用于“租户级”默认角色（要求 tenantID>0）
// - role_owner：租户所有者
// - role_admin：租户管理员
// - role_user ：租户普通用户
// - role_readonly：租户只读账号（演示/巡检）
func (r *RoleRepository) EnsureDefaultRoles(ctx context.Context, tenantUUID string) error {
	if strings.TrimSpace(tenantUUID) == "" {
		return fmt.Errorf("EnsureDefaultRoles: tenantUUID required")
	}

	defs := []dbm.Role{
		{Scope: string(iam.RoleScopeTenant), TenantUUID: tenantUUID, Code: iam.CodeRoleOwner, Name: "Tenant Owner", Builtin: true},
		{Scope: string(iam.RoleScopeTenant), TenantUUID: tenantUUID, Code: iam.CodeRoleAdmin, Name: "Tenant Admin", Builtin: true},
		{Scope: string(iam.RoleScopeTenant), TenantUUID: tenantUUID, Code: iam.CodeRoleUser, Name: "Tenant User", Builtin: true},
		{Scope: string(iam.RoleScopeTenant), TenantUUID: tenantUUID, Code: iam.CodeRoleReadonly, Name: "Tenant Readonly", Builtin: true},
	}
	for i := range defs {
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "scope"}, {Name: "tenant_uuid"}, {Name: "code"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				// 仅更新你希望“以代码为准”的字段（避免误覆盖）
				"name":    defs[i].Name,
				"builtin": true,
				// "description": defs[i].Description, // 如需同步描述再打开
			}),
		}).Create(&defs[i]).Error; err != nil {
			return err
		}
	}

	return nil
}

// ---------- 新增：分页筛选 ----------

type ListRoleOption struct {
	TenantUUID *string // 为 nil 表示 system-scope 查询
	Scope      string  // "tenant"|"system", 为空则不过滤
	Keyword    string  // 匹配 code/name
	Builtin    *bool
	Page       int
	PageSize   int
	Sort       string // e.g. "created_at desc", "name asc"
}

func (r *RoleRepository) FindPage(ctx context.Context, opt ListRoleOption) (items []dbm.Role, total int64, err error) {
	if opt.Page <= 0 {
		opt.Page = 1
	}
	if opt.PageSize <= 0 || opt.PageSize > 200 {
		opt.PageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&dbm.Role{})

	scope := strings.ToLower(strings.TrimSpace(opt.Scope))

	switch scope {
	case "tenant":
		if opt.TenantUUID == nil || strings.TrimSpace(*opt.TenantUUID) == "" {
			return nil, 0, errors.New("tenant scope requires tenant uuid")
		}
		q = q.Where("scope = 'tenant' AND tenant_uuid = ?", strings.TrimSpace(*opt.TenantUUID))
	case "system":
		q = q.Where("scope = 'system' AND (tenant_uuid = '' OR tenant_uuid IS NULL)")
	default:
		// 未指定 scope：
		// - 若指定了 tenant_uuid：只看该租户（tenant 角色）
		// - 若未指定 tenant_uuid：不加租户过滤（root 可看全量：system+所有租户）
		if opt.TenantUUID != nil && strings.TrimSpace(*opt.TenantUUID) != "" {
			q = q.Where("tenant_uuid = ?", strings.TrimSpace(*opt.TenantUUID)) // 只会命中 tenant 角色
		} else {
			// 不加 tenant 过滤 => root 场景看全量
		}
	}

	if kw := strings.TrimSpace(opt.Keyword); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		q = q.Where("(LOWER(code) LIKE ? OR LOWER(name) LIKE ?)", like, like)
	}
	if opt.Builtin != nil {
		q = q.Where("builtin = ?", *opt.Builtin)
	}

	if err = q.Count(&total).Error; err != nil {
		return
	}
	sort := "id DESC"
	if s := strings.TrimSpace(opt.Sort); s != "" {
		sort = s
	}
	err = q.Order(sort).
		Offset((opt.Page - 1) * opt.PageSize).
		Limit(opt.PageSize).
		Find(&items).Error
	return
}

// ---------- 新增：删除安全性检查（是否被绑定/授权） ----------

// CountBindings 统计 iam_role_binding 引用数
func (r *RoleRepository) CountBindings(ctx context.Context, tenantUUID *string, roleID uint64) (int64, error) {
	var n int64
	q := r.db.WithContext(ctx).Table("iam_role_binding").Where("role_id = ?", roleID)
	if tenantUUID != nil && strings.TrimSpace(*tenantUUID) != "" {
		q = q.Where("tenant_uuid = ?", strings.TrimSpace(*tenantUUID))
	}
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// CountPermissions 统计 iam_role_permission 引用数
func (r *RoleRepository) CountPermissions(ctx context.Context, roleID uint64) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Table("iam_role_permission").Where("role_id = ?", roleID).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// SafeDelete 删除前校验是否被使用；必须传当前租户（tenant 角色）或 nil（system 角色）
func (r *RoleRepository) SafeDelete(ctx context.Context, tenantUUID *string, roleID uint64) error {
	bindN, err := r.CountBindings(ctx, tenantUUID, roleID)
	if err != nil {
		return err
	}
	if bindN > 0 {
		return errors.New("role is bound and cannot be deleted")
	}
	permN, err := r.CountPermissions(ctx, roleID)
	if err != nil {
		return err
	}
	if permN > 0 {
		return errors.New("role has permissions; revoke first")
	}

	// 真正删除
	del := r.db.WithContext(ctx).Where("id = ?", roleID)
	if tenantUUID != nil && strings.TrimSpace(*tenantUUID) != "" {
		del = del.Where("tenant_uuid = ?", strings.TrimSpace(*tenantUUID))
	} else {
		del = del.Where("(tenant_uuid = '' OR tenant_uuid IS NULL)")
	}
	return del.Delete(&dbm.Role{}).Error
}
