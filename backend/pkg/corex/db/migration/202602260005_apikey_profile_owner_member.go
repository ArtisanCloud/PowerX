package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureAPIKeyProfileOwnerMemberMigration 为 iam_api_key_profile 增加 owner_member_id 字段。
func EnsureAPIKeyProfileOwnerMemberMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	table := coremodel.TableIAMAPIKeyProfile
	if !db.Migrator().HasTable(table) {
		return nil
	}
	if !db.Migrator().HasColumn(table, "owner_member_id") {
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN owner_member_id bigint`, table)).Error; err != nil {
			return fmt.Errorf("add column %s.owner_member_id failed: %w", table, err)
		}
	}
	if err := db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_owner_member_id ON %s(owner_member_id)`, table, table)).Error; err != nil {
		return fmt.Errorf("create owner_member_id index failed: %w", err)
	}
	return nil
}
