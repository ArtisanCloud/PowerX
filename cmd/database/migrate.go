// cmd/database/migrate.go
package main

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"gorm.io/gorm"
	"log"
)

func MigrateDatabase(ctx context.Context, db *gorm.DB) error {

	if err := database.MigrateCoreModels(db); err != nil {
		log.Fatalf("CoreX 迁移失败: %v", err)
	}

	return nil
}
