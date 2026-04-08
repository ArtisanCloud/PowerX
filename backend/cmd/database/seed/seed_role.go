// pkg/cmd/database/seed/role.go
package seed

import (
	"fmt"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
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
		if err := rpRepo.BindPermissions(seedCtx(), readonly.ID, readIDs...); err != nil {
			return fmt.Errorf("grant to readonly: %w", err)
		}
	}

	fmt.Printf("[seed] granted defaults for tenant=%s (admin:%d, user:%d, readonly:%d)\n", tenantUUID, len(allIDs), len(readIDs), len(readIDs))
	return nil
}
