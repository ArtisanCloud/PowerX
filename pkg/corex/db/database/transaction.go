package database

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Transaction 在事务中执行函数
func Transaction(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	// 开始事务
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("开始事务失败: %w", tx.Error)
	}

	// 执行函数
	if err := fn(tx); err != nil {
		// 回滚事务
		if rbErr := tx.Rollback().Error; rbErr != nil {
			return fmt.Errorf("执行失败 (%v) 且回滚失败: %w", err, rbErr)
		}
		return err
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}
