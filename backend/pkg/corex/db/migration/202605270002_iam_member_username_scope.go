package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureIAMMemberUsernameScopeMigration replaces the legacy global username
// uniqueness with tenant-scoped username uniqueness.
func EnsureIAMMemberUsernameScopeMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TableIAMMember
	if !db.Migrator().HasTable(table) {
		return nil
	}
	for _, name := range []string{"uni_iam_member_username", "uni_public_iam_member_username"} {
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s`, table, name)).Error; err != nil {
			return fmt.Errorf("drop legacy member username constraint %s failed: %w", name, err)
		}
		if err := db.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %s`, name)).Error; err != nil {
			return fmt.Errorf("drop legacy member username index %s failed: %w", name, err)
		}
	}
	if err := db.Exec(fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS uk_member_tenant_username ON %s(tenant_uuid, username)`, table)).Error; err != nil {
		return fmt.Errorf("create tenant-scoped member username index failed: %w", err)
	}
	return nil
}
