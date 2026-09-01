package database

import (
	"fmt"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modelrelease "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"gorm.io/gorm"
)

// migratePluginReleaseDeveloperMemberUUID performs the one-way conversion of
// local install sessions from the obsolete numeric developer_id to the IAM
// member UUID. It deliberately fails if historical rows cannot be mapped.
func migratePluginReleaseDeveloperMemberUUID(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("plugin release UUID migration requires database")
	}
	session := &modelrelease.LocalInstallSession{}
	if !db.Migrator().HasColumn(session, "developer_id") {
		return nil
	}
	sessionTable := session.TableName()
	memberTable := (&modeliam.Member{}).GetTableName(true)
	if err := db.Exec("UPDATE " + sessionTable + " SET developer_member_uuid = (SELECT uuid FROM " + memberTable + " AS m WHERE m.tenant_uuid = " + sessionTable + ".tenant_uuid AND m.id = " + sessionTable + ".developer_id) WHERE developer_member_uuid IS NULL").Error; err != nil {
		return fmt.Errorf("backfill plugin release developer_member_uuid: %w", err)
	}
	var missing int64
	if err := db.Table(sessionTable).Where("developer_member_uuid IS NULL").Count(&missing).Error; err != nil {
		return fmt.Errorf("validate plugin release developer_member_uuid: %w", err)
	}
	if missing != 0 {
		return fmt.Errorf("plugin release UUID migration blocked: %d sessions have no tenant member UUID for legacy developer_id", missing)
	}
	index := "idx_plugin_release_local_session_tenant"
	if db.Migrator().HasIndex(session, index) {
		if err := db.Migrator().DropIndex(session, index); err != nil {
			return fmt.Errorf("drop legacy plugin release developer index: %w", err)
		}
	}
	if err := db.Migrator().DropColumn(session, "developer_id"); err != nil {
		return fmt.Errorf("drop legacy plugin release developer_id: %w", err)
	}
	if err := db.Migrator().CreateIndex(session, index); err != nil {
		return fmt.Errorf("create plugin release member UUID index: %w", err)
	}
	return nil
}
