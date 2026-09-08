package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureIAMMemberActiveDisplayNameUniqueMigration makes display-name based
// business imports deterministic. Only active, non-deleted members participate;
// disabled historical members may retain their former display name.
func EnsureIAMMemberActiveDisplayNameUniqueMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	if db.Dialector.Name() != "postgres" {
		return fmt.Errorf("IAM active display-name uniqueness requires postgres")
	}
	table := coremodel.TableIAMMember
	if !db.Migrator().HasTable(table) {
		return nil
	}

	var duplicate struct {
		TenantUUID string
		NameKey    string
	}
	err := db.Raw(fmt.Sprintf(`
SELECT tenant_uuid, lower(trim(display_name)) AS name_key
FROM %s
WHERE status = 1 AND deleted_at IS NULL AND trim(display_name) <> ''
GROUP BY tenant_uuid, lower(trim(display_name))
HAVING COUNT(*) > 1
LIMIT 1`, table)).Scan(&duplicate).Error
	if err != nil {
		return fmt.Errorf("inspect IAM active display-name duplicates: %w", err)
	}
	if duplicate.TenantUUID != "" {
		return fmt.Errorf("IAM_MEMBER_DISPLAY_NAME_CONFLICT: resolve active duplicate display_name before migration tenant_uuid=%s display_name_key=%s", duplicate.TenantUUID, duplicate.NameKey)
	}
	if err := db.Exec(fmt.Sprintf(`
CREATE UNIQUE INDEX IF NOT EXISTS uk_iam_member_active_display_name
ON %s (tenant_uuid, lower(trim(display_name)))
WHERE status = 1 AND deleted_at IS NULL AND trim(display_name) <> ''`, table)).Error; err != nil {
		return fmt.Errorf("create IAM active display-name unique index: %w", err)
	}
	return nil
}
