package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureSkillsInstallTaskTenantUUIDMigration adds tenant_uuid for legacy skills_install_tasks tables.
func EnsureSkillsInstallTaskTenantUUIDMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TableSkillsInstallTasks
	if !db.Migrator().HasTable(table) {
		return nil
	}
	if !db.Migrator().HasColumn(table, "tenant_uuid") {
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN tenant_uuid char(36) NOT NULL DEFAULT ''`, table)).Error; err != nil {
			return fmt.Errorf("add column %s.tenant_uuid failed: %w", table, err)
		}
	}
	if err := db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_tenant_uuid ON %s(tenant_uuid)`, table, table)).Error; err != nil {
		return fmt.Errorf("create tenant_uuid index failed: %w", err)
	}
	return nil
}
