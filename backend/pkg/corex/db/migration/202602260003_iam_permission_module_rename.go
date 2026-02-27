package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureIAMPermissionModuleRenameMigration 把历史 plugin 列重命名为 module。
func EnsureIAMPermissionModuleRenameMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TableIAMPermission
	if !db.Migrator().HasTable(table) {
		return nil
	}
	hasPlugin := db.Migrator().HasColumn(table, "plugin")
	hasModule := db.Migrator().HasColumn(table, "module")
	if hasPlugin && !hasModule {
		if err := db.Migrator().RenameColumn(table, "plugin", "module"); err != nil {
			return fmt.Errorf("rename column %s.plugin -> module failed: %w", table, err)
		}
	}
	return nil
}
