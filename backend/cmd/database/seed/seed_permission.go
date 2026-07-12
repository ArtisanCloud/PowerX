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
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func permissionMeta(plugin, resource, action, typ string) []byte {
	if typ == "" {
		typ = "action"
	}
	m := map[string]any{
		"type":   typ,
		"module": plugin, // 让 catalog 里落在 plugin>action
		"label":  fmt.Sprintf("%s.%s.%s", plugin, resource, action),
	}
	b, _ := json.Marshal(m)
	return b
}

func mAction(plugin, resource, action string) []byte {
	return permissionMeta(plugin, resource, action, "action")
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

func menuPerm(resource string) dbm.Permission {
	permission := systemPerm("menu", resource, "read")
	permission.Description = fmt.Sprintf("Allow viewing admin menu %s", resource)
	permission.Meta = permissionMeta("menu", resource, "read", "menu")
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
		// Admin menu visibility permissions. These only control menu visibility;
		// route/API authorization must still be enforced by the target module.
		menuPerm("agent"),
		menuPerm("agent.chat"),
		menuPerm("agent.management"),
		menuPerm("agent.team"),
		menuPerm("agent.team_tasks"),
		menuPerm("agent.traces"),
		menuPerm("skills"),
		menuPerm("knowledge"),
		menuPerm("workflow"),
		menuPerm("media"),
		menuPerm("dashboard"),
		menuPerm("monitor"),
		menuPerm("plugins"),
		menuPerm("plugins.market"),
		menuPerm("plugins.subscriptions"),
		menuPerm("plugins.capabilities"),
		menuPerm("plugins.release"),
		menuPerm("settings"),
		menuPerm("settings.users"),
		menuPerm("settings.roles"),
		menuPerm("settings.config"),
		menuPerm("settings.ai"),
		menuPerm("settings.ai.model"),
		menuPerm("settings.ai.cost"),
		menuPerm("settings.ai.context_optimizer"),
		menuPerm("settings.integration_api_keys"),
	}

	// 你仓储里已有 UpsertBatch：幂等插入/更新
	if err := pr.UpsertBatch(seedCtx(), perms); err != nil {
		return fmt.Errorf("upsert system permissions: %w", err)
	}
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] system permissions ready: %d", len(perms))
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
	roleVendor, err := rr.FindByCode(ctx, "tenant", &tenantUUID, "role_vendor")
	if err != nil {
		return fmt.Errorf("find role_vendor: %w", err)
	}

	// 3) 计算权限集合
	var (
		allActiveIDs         []uint64 // root 全部
		readOnlyIDs          []uint64 // system_monitor 只读
		tenantActiveIDs      []uint64 // role_admin 租户可用
		tenantReadOnlyIDs    []uint64 // role_user 租户非菜单只读
		allMenuIDs           []uint64
		tenantDefaultMenuIDs []uint64 // role_user/role_readonly 默认菜单入口
		vendorIDs            []uint64 // role_vendor 供应商默认权限
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
			  AND module != 'menu'
			  AND action IN ('read','list')
		`, dbm.PermissionStatusActive, "system").
		Scan(&tenantReadOnlyIDs).Error; err != nil {
		return fmt.Errorf("list tenant readonly ids: %w", err)
	}

	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("status = ?", dbm.PermissionStatusActive).
		Where("module = ? AND action = ?", "menu", "read").
		Pluck("id", &allMenuIDs).Error; err != nil {
		return fmt.Errorf("list menu ids: %w", err)
	}

	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("status = ?", dbm.PermissionStatusActive).
		Where("module = ? AND resource IN ? AND action = ?",
			"menu",
			[]string{"dashboard", "agent", "agent.chat", "knowledge"},
			"read",
		).
		Pluck("id", &tenantDefaultMenuIDs).Error; err != nil {
		return fmt.Errorf("list tenant default menu ids: %w", err)
	}

	if err := db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("status = ?", dbm.PermissionStatusActive).
		Where("module = ? AND resource = ? AND action = ?", "iam", "permission", "read").
		Pluck("id", &vendorIDs).Error; err != nil {
		return fmt.Errorf("list vendor ids: %w", err)
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
		if len(tenantDefaultMenuIDs) > 0 {
			if err := syncRoleMenuPermissionsTx(tx, rpr, roleUser.ID, allMenuIDs, tenantDefaultMenuIDs); err != nil {
				return err
			}
			if err := syncRoleMenuPermissionsTx(tx, rpr, roleReadonly.ID, allMenuIDs, tenantDefaultMenuIDs); err != nil {
				return err
			}
		}
		if len(vendorIDs) > 0 {
			if err := rpr.GrantByIDsTx(tx, roleVendor.ID, vendorIDs); err != nil {
				return err
			}
		}
		return nil
	})
}

func syncRoleMenuPermissionsTx(tx *gorm.DB, rpr *infraiam.RolePermissionRepository, roleID uint64, allMenuIDs, allowedMenuIDs []uint64) error {
	if len(allMenuIDs) == 0 {
		return nil
	}
	allowed := make(map[uint64]struct{}, len(allowedMenuIDs))
	for _, id := range allowedMenuIDs {
		allowed[id] = struct{}{}
	}
	revokeIDs := make([]uint64, 0, len(allMenuIDs))
	for _, id := range allMenuIDs {
		if _, ok := allowed[id]; !ok {
			revokeIDs = append(revokeIDs, id)
		}
	}
	if len(revokeIDs) > 0 {
		if err := rpr.RevokeByIDsTx(tx, roleID, revokeIDs); err != nil {
			return err
		}
	}
	return rpr.GrantByIDsTx(tx, roleID, allowedMenuIDs)
}
