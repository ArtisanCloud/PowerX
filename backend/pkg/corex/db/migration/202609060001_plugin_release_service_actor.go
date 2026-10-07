package migration

import (
	"fmt"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsurePluginReleaseServiceActorMigration makes the install-session ownership
// contract explicit. A legacy developer-member session cannot be inferred as a
// plugin service actor, so existing rows fail migration with an actionable
// cleanup requirement instead of being silently reinterpreted.
func EnsurePluginReleaseServiceActorMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TablePluginReleaseLocalInstallSessions
	if schema := strings.TrimSpace(coremodel.PowerXSchema); schema != "" {
		table = schema + "." + table
	}
	if !db.Migrator().HasTable(table) {
		return nil
	}

	if db.Dialector.Name() == "postgres" {
		parts := strings.Split(table, ".")
		if len(parts) != 2 {
			return fmt.Errorf("invalid plugin release table name: %s", table)
		}
		ref := fmt.Sprintf(`"%s"."%s"`, strings.Trim(parts[0], `"`), strings.Trim(parts[1], `"`))
		for _, statement := range []string{
			fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS plugin_id varchar(255)`, ref),
			fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS service_actor varchar(255)`, ref),
		} {
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("prepare plugin release service actor columns: %w", err)
			}
		}
	} else if db.Dialector.Name() == "sqlite" {
		for _, statement := range []string{
			fmt.Sprintf(`ALTER TABLE %s ADD COLUMN plugin_id text`, table),
			fmt.Sprintf(`ALTER TABLE %s ADD COLUMN service_actor text`, table),
		} {
			if err := db.Exec(statement).Error; err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				return fmt.Errorf("prepare plugin release service actor columns: %w", err)
			}
		}
	}

	var legacyCount int64
	if err := db.Table(table).Where("plugin_id IS NULL OR trim(plugin_id) = '' OR service_actor IS NULL OR trim(service_actor) = ''").Count(&legacyCount).Error; err != nil {
		return fmt.Errorf("inspect plugin release service actor migration: %w", err)
	}
	if legacyCount != 0 {
		return fmt.Errorf("plugin release service-actor migration blocked: %d legacy install session(s) lack plugin_id/service_actor; archive or delete those historical rows before retrying migrate", legacyCount)
	}

	if db.Dialector.Name() == "postgres" {
		parts := strings.Split(table, ".")
		ref := fmt.Sprintf(`"%s"."%s"`, strings.Trim(parts[0], `"`), strings.Trim(parts[1], `"`))
		for _, statement := range []string{
			fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN plugin_id SET NOT NULL`, ref),
			fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN service_actor SET NOT NULL`, ref),
			fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS uk_plugin_release_active_session_tenant_plugin ON %s (tenant_uuid, plugin_id) WHERE status = 'in_progress'`, ref),
		} {
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("finalize plugin release service actor migration: %w", err)
			}
		}
	}
	return nil
}
