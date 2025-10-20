package migration

import (
	"fmt"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"gorm.io/gorm"
)

// CreateEventAuthorizationTables 构建授权域相关核心表。
func CreateEventAuthorizationTables(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	if err := db.AutoMigrate(
		&eventfabricmodel.AuthorizationCapability{},
		&eventfabricmodel.AuthorizationGrant{},
		&eventfabricmodel.AuthorizationGrantCapability{},
		&eventfabricmodel.AuthorizationGrantCondition{},
		&eventfabricmodel.AuthorizationApprovalTicket{},
	); err != nil {
		return err
	}

	// 兼容历史版本，补充必要索引（AutoMigrate 会处理大部分结构）。
	if err := db.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS uk_event_auth_capability ON " +
			(&eventfabricmodel.AuthorizationCapability{}).TableName() +
			" (namespace, action);",
	).Error; err != nil {
		return err
	}

	if err := db.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS uk_event_auth_grant_subject_status_active ON " +
			(&eventfabricmodel.AuthorizationGrant{}).TableName() +
			" (tenant_id, subject_type, subject_id) " +
			"WHERE status IN ('pending','active');",
	).Error; err != nil {
		return err
	}

	if err := db.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS uk_event_auth_grant_capability ON " +
			(&eventfabricmodel.AuthorizationGrantCapability{}).TableName() +
			" (grant_id, capability_id);",
	).Error; err != nil {
		return err
	}

	if err := db.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS uk_event_auth_grant_condition ON " +
			(&eventfabricmodel.AuthorizationGrantCondition{}).TableName() +
			" (grant_id, type, expression);",
	).Error; err != nil {
		return err
	}

	if err := db.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS uk_event_auth_ticket_fingerprint ON " +
			(&eventfabricmodel.AuthorizationApprovalTicket{}).TableName() +
			" (request_fingerprint);",
	).Error; err != nil {
		return err
	}

	return nil
}
