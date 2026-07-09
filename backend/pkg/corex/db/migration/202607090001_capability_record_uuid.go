package migration

import (
	"context"
	"fmt"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EnsureCapabilityRecordUUIDMigration backfills UUIDs for existing capability records.
func EnsureCapabilityRecordUUIDMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	ctx := context.Background()
	var rows []models.CapabilityRecord
	if err := db.WithContext(ctx).
		Model(&models.CapabilityRecord{}).
		Where("uuid IS NULL OR uuid = ?", uuid.Nil).
		Find(&rows).Error; err != nil {
		return fmt.Errorf("list capability records missing uuid: %w", err)
	}
	for i := range rows {
		nextUUID := uuid.New()
		if err := db.WithContext(ctx).
			Model(&models.CapabilityRecord{}).
			Where("id = ?", rows[i].ID).
			Update("uuid", nextUUID).Error; err != nil {
			return fmt.Errorf("backfill capability record uuid id=%d: %w", rows[i].ID, err)
		}
	}
	return nil
}
