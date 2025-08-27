// pkg/cmd/database/seed/role.go
package seed

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	tenantRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
)

// 说明：
// 1) System 级别角色（tenant_id = NULL），用于平台级 root/只读等
// 2) Tenant 级别角色（每个租户各自一套），通过 RoleRepository.EnsureDefaultRoles 保证
// 3) 统一使用 (scope, tenant_id, code) 唯一约束，OnConflict DoNothing 幂等插入

// SeedSystemBuiltinRoles 确保系统级（scope=system, tenant_id=NULL）的内置角色
// - system_admin：系统最高权限（不可删除）
// - system_monitor：系统只读（可选，按需保留）
func SeedSystemBuiltinRoles(db *gorm.DB) error {
	sysRoles := []dbm.Role{
		{Scope: string(iam.RoleScopeSystem), TenantID: iam.SystemTenantID, Code: iam.CodeSystemAdmin, Name: "System Admin", Builtin: true},
		{Scope: string(iam.RoleScopeSystem), TenantID: iam.SystemTenantID, Code: iam.CodeSystemMonitor, Name: "System Monitor", Builtin: true},
	}
	return db.WithContext(seedCtx()).Transaction(func(tx *gorm.DB) error {
		for i := range sysRoles {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "scope"}, {Name: "tenant_id"}, {Name: "code"}},
				DoNothing: true,
			}).Create(&sysRoles[i]).Error; err != nil {
				return fmt.Errorf("seed system role(code=%s): %w", sysRoles[i].Code, err)
			}
		}
		fmt.Printf("[seed] system builtin roles ready: %d\n", len(sysRoles))
		return nil
	})
}

// SeedTenantBuiltinRoles 为指定租户（通过 tenantKey 定位）确保租户级内置角色
// - role_admin（Tenant Admin）：本租户最高权限
// - role_user（Tenant User）：普通用户基线
// 注：你已经在 RoleRepository 中实现了 EnsureDefaultRoles，这里直接复用。
func SeedTenantBuiltinRoles(db *gorm.DB, tenantKey string) error {
	tenRepo := tenantRepo.NewTenantRepository(db)
	roleRepo := infraiam.NewRoleRepository(db)

	// 1) 确保租户存在
	ten, err := tenRepo.EnsureByKey(seedCtx(), tenantKey, "Seeded Tenant")
	if err != nil {
		return fmt.Errorf("ensure tenant(%s): %w", tenantKey, err)
	}

	// 2) 调用你现有的 EnsureDefaultRoles（role_admin / role_user）
	if err := roleRepo.EnsureDefaultRoles(seedCtx(), ten.ID); err != nil {
		return fmt.Errorf("ensure default roles for tenant(%s,id=%d): %w", tenantKey, ten.ID, err)
	}

	fmt.Printf("[seed] tenant builtin roles ready for tenant=%s (id=%d)\n", tenantKey, ten.ID)
	return nil
}

// SeedAllBuiltinRoles 一次性为“系统 + 多个租户”落下内置角色
func SeedAllBuiltinRoles(db *gorm.DB, tenantKeys []string) error {
	if err := SeedSystemBuiltinRoles(db); err != nil {
		return err
	}
	for _, k := range tenantKeys {
		if err := SeedTenantBuiltinRoles(db, k); err != nil {
			return err
		}
	}
	return nil
}

// SeedGrantDefaultRolesForTenant：为指定租户把权限授予给内置角色（admin=全量，user=只读）
func SeedGrantDefaultRolesForTenant(db *gorm.DB, tenantID uint64) error {
	roleRepo := infraiam.NewRoleRepository(db)
	rpRepo := infraiam.NewRolePermissionRepository(db)

	// 1) 确保租户默认角色存在
	if err := roleRepo.EnsureDefaultRoles(seedCtx(), tenantID); err != nil {
		return fmt.Errorf("ensure default roles: %w", err)
	}
	admin, err := roleRepo.FindByCode(seedCtx(), "tenant", &tenantID, "role_admin")
	if err != nil {
		return fmt.Errorf("find role_admin: %w", err)
	}
	user, err := roleRepo.FindByCode(seedCtx(), "tenant", &tenantID, "role_user")
	if err != nil {
		return fmt.Errorf("find role_user: %w", err)
	}

	// 2) 查全量和只读权限 ID（全局 permission 表）
	var allIDs, readIDs []uint64
	if err := db.WithContext(seedCtx()).
		Model(&dbm.Permission{}).Pluck("id", &allIDs).Error; err != nil {
		return err
	}
	if err := db.WithContext(seedCtx()).
		Model(&dbm.Permission{}).
		Where("action = ?", "read").
		Pluck("id", &readIDs).Error; err != nil {
		return err
	}

	// 3) 授予（幂等 ON CONFLICT DO NOTHING）
	if len(allIDs) > 0 {
		if err := rpRepo.BindPermissions(seedCtx(), admin.ID, allIDs...); err != nil {
			return fmt.Errorf("grant to admin: %w", err)
		}
	}
	if len(readIDs) > 0 {
		if err := rpRepo.BindPermissions(seedCtx(), user.ID, readIDs...); err != nil {
			return fmt.Errorf("grant to user: %w", err)
		}
	}

	fmt.Printf("[seed] granted defaults for tenant=%d (admin:%d, user:%d)\n", tenantID, len(allIDs), len(readIDs))
	return nil
}
