package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureIAMPermissionAllowAPIKeyMigration 为 iam_permission 增加 allow_api_key 字段并回填历史 API Key 权限。
func EnsureIAMPermissionAllowAPIKeyMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TableIAMPermission
	if !db.Migrator().HasTable(table) {
		return nil
	}
	if !db.Migrator().HasColumn(table, "allow_api_key") {
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN allow_api_key boolean NOT NULL DEFAULT false`, table)).Error; err != nil {
			return fmt.Errorf("add column %s.allow_api_key failed: %w", table, err)
		}
	}
	if err := db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_allow_api_key ON %s(allow_api_key)`, table, table)).Error; err != nil {
		return fmt.Errorf("create allow_api_key index failed: %w", err)
	}
	// 历史兼容：旧模板统一用 api_key.* resource，迁移后自动标记为 allow_api_key=true。
	if err := db.Exec(fmt.Sprintf(`UPDATE %s SET allow_api_key = true WHERE resource LIKE 'api_key.%%'`, table)).Error; err != nil {
		return fmt.Errorf("backfill allow_api_key failed: %w", err)
	}
	return nil
}
