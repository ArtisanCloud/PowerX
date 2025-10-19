package migration

import (
	"fmt"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"gorm.io/gorm"
)

// CreateEventDeliveryTables 创建可靠投递所需的核心表。
func CreateEventDeliveryTables(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	if err := db.AutoMigrate(
		&eventfabricmodel.EventEnvelope{},
		&eventfabricmodel.DeliveryAttempt{},
		&eventfabricmodel.DlqMessage{},
		&eventfabricmodel.SubscriptionOffset{},
	); err != nil {
		return err
	}

	// 额外索引（AutoMigrate 已处理大部分索引，此处补充复合索引）。
	if err := db.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS uk_event_delivery_attempt_ident ON " +
			(&eventfabricmodel.DeliveryAttempt{}).TableName() +
			" (tenant_key, event_id, subscriber_id, delivery_no);",
	).Error; err != nil {
		return err
	}

	if err := db.Exec(
		"CREATE UNIQUE INDEX IF NOT EXISTS uk_event_envelopes_tenant_event ON " +
			(&eventfabricmodel.EventEnvelope{}).TableName() +
			" (tenant_key, event_id);",
	).Error; err != nil {
		return err
	}

	return nil
}
