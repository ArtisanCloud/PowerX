package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureIAMUserLastTenantUUIDMigration adds the persisted last tenant pointer
// used to select a default tenant during password login.
func EnsureIAMUserLastTenantUUIDMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TableIAMUser
	if !db.Migrator().HasTable(table) {
		return nil
	}
	if !db.Migrator().HasColumn(table, "last_tenant_uuid") {
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN last_tenant_uuid char(36) NOT NULL DEFAULT ''`, table)).Error; err != nil {
			return fmt.Errorf("add %s.last_tenant_uuid failed: %w", table, err)
		}
	}
	if err := db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_last_tenant_uuid ON %s(last_tenant_uuid)`, table, table)).Error; err != nil {
		return fmt.Errorf("create last_tenant_uuid index failed: %w", err)
	}
	return nil
}
