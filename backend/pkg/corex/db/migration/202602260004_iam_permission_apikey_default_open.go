package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureIAMPermissionAPIKeyDefaultOpenMigration 将权限默认开放给 API Key，并对核心敏感接口回收。
func EnsureIAMPermissionAPIKeyDefaultOpenMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TableIAMPermission
	if !db.Migrator().HasTable(table) || !db.Migrator().HasColumn(table, "allow_api_key") {
		return nil
	}
	if err := db.Exec(fmt.Sprintf(`UPDATE %s SET allow_api_key = true WHERE status = 'active'`, table)).Error; err != nil {
		return fmt.Errorf("set allow_api_key default open failed: %w", err)
	}
	if err := db.Exec(fmt.Sprintf(`
		UPDATE %s
		SET allow_api_key = false
		WHERE
			module = 'system'
			OR (module = 'iam' AND resource = 'credential')
			OR (module = 'iam' AND resource = 'permission' AND action NOT IN ('read','list'))
			OR COALESCE(meta->>'api_endpoint','') LIKE '/api/v1/admin/user/auth/%%'
			OR COALESCE(meta->>'api_endpoint','') LIKE '/api/v1/admin/integration/api-keys%%'
			OR COALESCE(meta->>'api_endpoint','') LIKE '/api/v1/admin/integration/api-key-profiles%%'
			OR COALESCE(meta->>'api_endpoint','') LIKE '/api/v1/admin/iam/permissions%%'
			OR COALESCE(meta->>'api_endpoint','') LIKE '/api/v1/admin/iam/roles%%'
	`, table)).Error; err != nil {
		return fmt.Errorf("set allow_api_key sensitive deny failed: %w", err)
	}
	return nil
}
