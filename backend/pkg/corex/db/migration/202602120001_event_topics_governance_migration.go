package migration

import (
	"fmt"

	"gorm.io/gorm"
)

// EnsureEventTopicsGovernanceMigration 在首版开发阶段保持 no-op。
// 当前约定：由 AutoMigrate 直接按最新模型建表，不执行任何增量兼容迁移。
func EnsureEventTopicsGovernanceMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	return nil
}

// RollbackEventTopicsGovernanceMigration 在首版开发阶段保持 no-op。
func RollbackEventTopicsGovernanceMigration(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	return nil
}
