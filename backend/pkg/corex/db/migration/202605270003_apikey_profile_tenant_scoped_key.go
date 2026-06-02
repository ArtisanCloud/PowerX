package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureAPIKeyProfileTenantScopedKeyMigration removes legacy global API key
// profile key uniqueness and keeps profile keys unique only inside a tenant.
func EnsureAPIKeyProfileTenantScopedKeyMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TableIAMAPIKeyProfile
	if !db.Migrator().HasTable(table) {
		return nil
	}
	for _, name := range []string{
		"idx_public_iam_api_key_profile_uk_svc_tenant_key",
		"idx_iam_api_key_profile_uk_svc_tenant_key",
		"uni_iam_api_key_profile_key",
		"uni_public_iam_api_key_profile_key",
	} {
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s`, table, name)).Error; err != nil {
			return fmt.Errorf("drop legacy api key profile constraint %s failed: %w", name, err)
		}
		if err := db.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %s`, name)).Error; err != nil {
			return fmt.Errorf("drop legacy api key profile index %s failed: %w", name, err)
		}
	}
	if err := db.Exec(fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS uk_svc_tenant_key ON %s(tenant_uuid, key)`, table)).Error; err != nil {
		return fmt.Errorf("create tenant-scoped api key profile index failed: %w", err)
	}
	return nil
}
