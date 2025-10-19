package migration

import (
	"fmt"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"gorm.io/gorm"
)

// CreateEventReplayTables 创建回放相关表。
func CreateEventReplayTables(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	return db.AutoMigrate(&eventfabricmodel.ReplayRequest{})
}
