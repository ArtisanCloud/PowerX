package migration

import (
	"fmt"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureIAMTenantDomainBackfillMigration fills the required tenant domain for
// legacy tenants created before SaaS signup generated tenant domains.
func EnsureIAMTenantDomainBackfillMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TableIAMTenant
	if !db.Migrator().HasTable(table) || !db.Migrator().HasColumn(table, "domain") {
		return nil
	}
	type tenantRow struct {
		ID     uint64
		Key    string
		Domain string
	}
	var rows []tenantRow
	if err := db.Table(table).Select("id, key, domain").Where("deleted_at IS NULL").Find(&rows).Error; err != nil {
		return fmt.Errorf("list tenant domains failed: %w", err)
	}
	used := make(map[string]uint64, len(rows))
	for _, row := range rows {
		domain := strings.ToLower(strings.TrimSpace(row.Domain))
		if domain == "" {
			continue
		}
		if existedID, ok := used[domain]; ok && existedID != row.ID {
			return fmt.Errorf("duplicate tenant domain %q on tenant ids %d and %d", domain, existedID, row.ID)
		}
		used[domain] = row.ID
	}
	for _, row := range rows {
		if strings.TrimSpace(row.Domain) != "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(row.Key))
		if key == "" {
			return fmt.Errorf("tenant %d has empty key and domain", row.ID)
		}
		domain := key + ".tenant.powerx.local"
		if existedID, ok := used[domain]; ok && existedID != row.ID {
			return fmt.Errorf("generated tenant domain %q for tenant id %d conflicts with tenant id %d", domain, row.ID, existedID)
		}
		if err := db.Table(table).Where("id = ?", row.ID).Update("domain", domain).Error; err != nil {
			return fmt.Errorf("backfill tenant %d domain failed: %w", row.ID, err)
		}
		used[domain] = row.ID
	}
	return nil
}
