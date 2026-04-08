package seed

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ArtisanCloud/PowerX/config"
	apikeypermissions "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

func mAction(plugin, resource, action string) []byte {
	m := map[string]any{
		"type":   "action",
		"module": plugin, // 让 catalog 里落在 plugin>action
		"label":  fmt.Sprintf("%s.%s.%s", plugin, resource, action),
	}
	b, _ := json.Marshal(m)
	return b
}

func systemPerm(module, resource, action string) dbm.Permission {
	permission := dbm.Permission{
		Module:     module,
		Resource:   resource,
		Action:     action,
		Source:     "core",
		Introduced: config.GetSystemVersion(),
		Meta:       mAction(module, resource, action),
	}
	permission.AllowAPIKey = apikeypermissions.DefaultAllowAPIKey(permission)
	return permission
}

// SeedSystemPermissions：把核心模块(IAM)的一批常用权限写入 iam_permission（幂等）
func SeedSystemPermissions(db *gorm.DB) error {
	pr := infraiam.NewPermissionRepository(db)

	perms := []dbm.Permission{
		// IAM / Role
		systemPerm("iam", "role", "read"),
		systemPerm("iam", "role", "write"),
		systemPerm("iam", "role", "delete"),
		systemPerm("iam", "role", "bind"),
		// IAM / User
		systemPerm("iam", "user", "read"),
		systemPerm("iam", "user", "write"),
		systemPerm("iam", "user", "delete"),
		// IAM / Department
		systemPerm("iam", "department", "read"),
		systemPerm("iam", "department", "write"),
		systemPerm("iam", "department", "delete"),
		// IAM / Permission（只读）
		systemPerm("iam", "permission", "read"),
		// Admin root guard（用于开放市场/发布候选菜单）
		systemPerm("admin", "root", "view"),
	}

	// 你仓储里已有 UpsertBatch：幂等插入/更新
	if err := pr.UpsertBatch(seedCtx(), perms); err != nil {
		return fmt.Errorf("upsert system permissions: %w", err)
	}
	fmt.Printf("[seed] system permissions ready: %d\n", len(perms))
	return nil
}

func SeedBuiltInRolesAndGrants(db *gorm.DB, tenantUUID string) error {
	rr := infraiam.NewRoleRepository(db)
	rpr := infraiam.NewRolePermissionRepository(db)
	ctx := context.Background()

	// 1) upsert 系统级角色：root、system_monitor（tenant_id=0）
	rootOut, err := rr.Upsert(ctx, &dbm.Role{
		Scope:      "system",
		TenantUUID: "",
		Code:       "root",
		Name:       "Super Admin",
		Builtin:    true,
	}, []clause.Column{{Name: "scope"}, {Name: "tenant_uuid"}, {Name: "code"}})
	if err != nil {
		return fmt.Errorf("upsert root: %w", err)
	}
	monitorOut, err := rr.Upsert(ctx, &dbm.Role{
		Scope:      "system",
		TenantUUID: "",
		Code:       "system_monitor",
		Name:       "System Monitor",
		Builtin:    true,
	}, []clause.Column{{Name: "scope"}, {Name: "tenant_uuid"}, {Name: "code"}})
	if err != nil {
		return fmt.Errorf("upsert system_monitor: %w", err)
	}

	// 2) 确保租户默认角色（role_admin / role_user / role_readonly）
	if err := rr.EnsureDefaultRoles(ctx, tenantUUID); err != nil {
		return fmt.Errorf("ensure default roles: %w", err)
	}
	roleAdmin, err := rr.FindByCode(ctx, "tenant", &tenantUUID, "role_admin")
	if err != nil {
		return fmt.Errorf("find role_admin: %w", err)
	}
	roleUser, err := rr.FindByCode(ctx, "tenant", &tenantUUID, "role_user")
	if err != nil {
		return fmt.Errorf("find role_user: %w", err)
	}
	roleReadonly, err := rr.FindByCode(ctx, "tenant", &tenantUUID, "role_readonly")
	if err != nil {
		return fmt.Errorf("find role_readonly: %w", err)
	}

	// 3) 计算权限集合
	var (
		allActiveIDs      []uint64 // root 全部
		readOnlyIDs       []uint64 // system_monitor 只读
		tenantActiveIDs   []uint64 // role_admin 租户可用
		tenantReadOnlyIDs []uint64 // role_user 租户只读
	)

	// root：所有 active
	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("status = ?", dbm.PermissionStatusActive).
		Pluck("id", &allActiveIDs).Error; err != nil {
		return fmt.Errorf("list all active ids: %w", err)
	}

	// system_monitor：全局只读（read/list）
	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("status = ?", dbm.PermissionStatusActive).
		Where("action IN ?", []string{"read", "list"}).
		Pluck("id", &readOnlyIDs).Error; err != nil {
		return fmt.Errorf("list readonly ids: %w", err)
	}

	// role_admin：租户可用（排除 module=system 的 API）
	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("status = ?", dbm.PermissionStatusActive).
		Where("(meta->>'module') IS NULL OR (meta->>'module') != ?", "system").
		Pluck("id", &tenantActiveIDs).Error; err != nil {
		return fmt.Errorf("list tenant active ids: %w", err)
	}

	// role_user：租户只读（租户可用 + read/list）
	if err := db.WithContext(ctx).
		Raw(`
			SELECT id FROM public.iam_permission
			WHERE status = ?
			  AND (meta->>'module' IS NULL OR (meta->>'module') != ?)
			  AND action IN ('read','list')
		`, dbm.PermissionStatusActive, "system").
		Scan(&tenantReadOnlyIDs).Error; err != nil {
		return fmt.Errorf("list tenant readonly ids: %w", err)
	}

	// 4) 授权（幂等）
	return db.Transaction(func(tx *gorm.DB) error {
		if len(allActiveIDs) > 0 {
			if err := rpr.GrantByIDsTx(tx, rootOut.ID, allActiveIDs); err != nil {
				return err
			}
		}
		if len(readOnlyIDs) > 0 {
			if err := rpr.GrantByIDsTx(tx, monitorOut.ID, readOnlyIDs); err != nil {
				return err
			}
		}
		if len(tenantActiveIDs) > 0 {
			if err := rpr.GrantByIDsTx(tx, roleAdmin.ID, tenantActiveIDs); err != nil {
				return err
			}
		}
		if len(tenantReadOnlyIDs) > 0 {
			if err := rpr.GrantByIDsTx(tx, roleUser.ID, tenantReadOnlyIDs); err != nil {
				return err
			}
			if err := rpr.GrantByIDsTx(tx, roleReadonly.ID, tenantReadOnlyIDs); err != nil {
				return err
			}
		}
		return nil
	})
}
