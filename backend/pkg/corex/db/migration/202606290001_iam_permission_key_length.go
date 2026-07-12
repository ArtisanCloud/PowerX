package migration

import (
	"fmt"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureIAMPermissionKeyLengthMigration widens permission identity columns for
// plugin resources such as menu/capability keys.
func EnsureIAMPermissionKeyLengthMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TableIAMPermission
	if !db.Migrator().HasTable(table) {
		return nil
	}

	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		schema = "public"
	}
	tableRef := fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent(table))
	for _, column := range []string{"module", "resource", "action"} {
		if !db.Migrator().HasColumn(table, column) {
			continue
		}
		stmt := fmt.Sprintf(
			`ALTER TABLE %s ALTER COLUMN %s TYPE varchar(255)`,
			tableRef,
			quoteIdent(column),
		)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("widen %s.%s failed: %w", table, column, err)
		}
	}
	return nil
}
