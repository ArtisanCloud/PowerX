package migrate

import (
	"context"
	"gorm.io/gorm"
)

func AutoMigrateCustom(ctx context.Context, db *gorm.DB) {
	// migrate your custom models
}
