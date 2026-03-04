package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureAPIKeyProfileNamingMigration 把历史命名(service_account/service_id)无损迁移到 profile 命名。
func EnsureAPIKeyProfileNamingMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}

	const oldProfileTable = "iam_service_account"
	newProfileTable := coremodel.TableIAMAPIKeyProfile
	if err := renameTableIfNeeded(db, oldProfileTable, newProfileTable); err != nil {
		return err
	}

	if err := renameColumnIfNeeded(db, coremodel.TableIAMAPIKey, "service_id", "profile_id"); err != nil {
		return err
	}
	if err := renameColumnIfNeeded(db, coremodel.TableIntegrationGatewayAPIKey, "service_id", "profile_id"); err != nil {
		return err
	}
	return nil
}

func renameTableIfNeeded(db *gorm.DB, oldName string, newName string) error {
	if oldName == "" || newName == "" || oldName == newName {
		return nil
	}
	oldExists := db.Migrator().HasTable(oldName)
	newExists := db.Migrator().HasTable(newName)
	if !oldExists || newExists {
		return nil
	}
	if err := db.Migrator().RenameTable(oldName, newName); err != nil {
		return fmt.Errorf("rename table %s -> %s failed: %w", oldName, newName, err)
	}
	return nil
}

func renameColumnIfNeeded(db *gorm.DB, tableName string, oldCol string, newCol string) error {
	if tableName == "" || oldCol == "" || newCol == "" || oldCol == newCol {
		return nil
	}
	hasOld := db.Migrator().HasColumn(tableName, oldCol)
	hasNew := db.Migrator().HasColumn(tableName, newCol)
	if !hasOld || hasNew {
		return nil
	}
	if err := db.Migrator().RenameColumn(tableName, oldCol, newCol); err != nil {
		return fmt.Errorf("rename column %s.%s -> %s failed: %w", tableName, oldCol, newCol, err)
	}
	return nil
}
