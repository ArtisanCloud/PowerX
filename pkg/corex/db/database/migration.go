package database

import (
	"fmt"
	modelForm "github.com/ArtisanCloud/PowerX/pkg/dynamic_form/persistence/model"

	"gorm.io/gorm"
)

// Migrate 执行数据库迁移
func MigrateCoreModels(db *gorm.DB) error {
	// 迁移动态表单
	if err := modelForm.AutoMigrate(db); err != nil {
		return fmt.Errorf("自动迁移失败: %w", err)
	}
	return nil
}
