// cmd/database/migrate.go
package main

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/persistance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
	"log"
)

func MigrateDatabase(ctx context.Context, db *gorm.DB) error {

	if err := database.MigrateCoreModels(db); err != nil {
		log.Fatalf("CoreX 迁移失败: %v", err)
	}

	if err := persistance.MigrateAgentModels(db); err != nil {
		log.Fatalf("Agent 迁移失败: %v", err)
	}

	return nil
}

func ResetDatabase(ctx context.Context, db *gorm.DB) error {
	// 如果你用 GORM，可以直接 drop 所有表
	// 或者先获取表名，再循环 drop
	// 这里举例简单版本：
	err := db.Exec("DROP SCHEMA " + model.PowerXSchema + " CASCADE; CREATE SCHEMA " + model.PowerXSchema + ";").Error
	if err != nil {
		return err
	}
	return nil
}
