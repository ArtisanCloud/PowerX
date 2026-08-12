// pkg/cmd/database/seed/role.go
package seed

import (
	"context"
	"fmt"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

// SeedGrantDefaultRolesForTenant：为指定租户把权限授予给内置角色（admin=全量，user/readonly=只读）
func SeedGrantDefaultRolesForTenant(db *gorm.DB, tenantUUID string) error {
	roleRepo := infraiam.NewRoleRepository(db)
	rpRepo := infraiam.NewRolePermissionRepository(db)

	// 1) 确保租户默认角色存在
	if err := roleRepo.EnsureDefaultRoles(seedCtx(), tenantUUID); err != nil {
		return fmt.Errorf("ensure default roles: %w", err)
	}
	admin, err := roleRepo.FindByCode(seedCtx(), "tenant", &tenantUUID, "role_admin")
	if err != nil {
		return fmt.Errorf("find role_admin: %w", err)
	}
	user, err := roleRepo.FindByCode(seedCtx(), "tenant", &tenantUUID, "role_user")
	if err != nil {
		return fmt.Errorf("find role_user: %w", err)
	}
	readonly, err := roleRepo.FindByCode(seedCtx(), "tenant", &tenantUUID, "role_readonly")
	if err != nil {
		return fmt.Errorf("find role_readonly: %w", err)
	}
	vendor, err := roleRepo.FindByCode(seedCtx(), "tenant", &tenantUUID, "role_vendor")
	if err != nil {
		return fmt.Errorf("find role_vendor: %w", err)
	}

	// 2) 查全量、普通用户基础权限和只读权限 ID（全局 permission 表）
	var allIDs, userIDs, readIDs, allMenuIDs, tenantDefaultMenuIDs, vendorIDs []uint64
	if err := db.WithContext(seedCtx()).
		Model(&dbm.Permission{}).Pluck("id", &allIDs).Error; err != nil {
		return err
	}
	if err := db.WithContext(seedCtx()).
		Model(&dbm.Permission{}).
		Where("action = ?", "read").
		Where("module <> ?", "menu").
		Pluck("id", &readIDs).Error; err != nil {
		return err
	}
	if err := db.WithContext(seedCtx()).
		Model(&dbm.Permission{}).
		Where("status = ?", dbm.PermissionStatusActive).
		Where("module <> ?", "menu").
		Where("action IN ? OR module = ?", []string{"read", "list"}, "agent").
		Pluck("id", &userIDs).Error; err != nil {
		return err
	}
	if err := db.WithContext(seedCtx()).
		Model(&dbm.Permission{}).
		Where("module = ? AND action = ?", "menu", "view").
		Pluck("id", &allMenuIDs).Error; err != nil {
		return err
	}
	if err := db.WithContext(seedCtx()).
		Model(&dbm.Permission{}).
		Where("module = ? AND resource IN ? AND action = ?",
			"menu",
			[]string{"dashboard", "agent", "agent.chat", "knowledge"},
			"view",
		).
		Pluck("id", &tenantDefaultMenuIDs).Error; err != nil {
		return err
	}
	if err := db.WithContext(seedCtx()).
		Model(&dbm.Permission{}).
		Where("module = ? AND resource = ? AND action = ?", "iam", "permission", "read").
		Pluck("id", &vendorIDs).Error; err != nil {
		return err
	}

	// 3) 授予（幂等 ON CONFLICT DO NOTHING）
	if len(allIDs) > 0 {
		if err := rpRepo.BindPermissions(seedCtx(), admin.ID, allIDs...); err != nil {
			return fmt.Errorf("grant to admin: %w", err)
		}
	}
	if len(userIDs) > 0 {
		if err := rpRepo.BindPermissions(seedCtx(), user.ID, userIDs...); err != nil {
			return fmt.Errorf("grant to user: %w", err)
		}
	}
	if len(readIDs) > 0 {
		if err := rpRepo.BindPermissions(seedCtx(), readonly.ID, readIDs...); err != nil {
			return fmt.Errorf("grant to readonly: %w", err)
		}
	}
	if len(tenantDefaultMenuIDs) > 0 {
		if err := syncRoleMenuPermissionsTx(db, rpRepo, user.ID, allMenuIDs, tenantDefaultMenuIDs); err != nil {
			return fmt.Errorf("grant default menu to user: %w", err)
		}
		if err := syncRoleMenuPermissionsTx(db, rpRepo, readonly.ID, allMenuIDs, tenantDefaultMenuIDs); err != nil {
			return fmt.Errorf("grant default menu to readonly: %w", err)
		}
	}
	if len(vendorIDs) > 0 {
		if err := rpRepo.BindPermissions(seedCtx(), vendor.ID, vendorIDs...); err != nil {
			return fmt.Errorf("grant to vendor: %w", err)
		}
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] granted defaults for tenant=%s (admin:%d, user:%d, readonly:%d, vendor:%d)", tenantUUID, len(allIDs), len(userIDs), len(readIDs), len(vendorIDs))
	return nil
}
