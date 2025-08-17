// Package auth 提供认证和授权功能
package auth

import (
	"fmt"
	"os"
	"time"
)

// Init 初始化认证系统
func Init() error {
	// 从环境变量获取JWT密钥
	secret := os.Getenv("CORE_X_JWT_SECRET")
	if secret == "" {
		// 开发环境使用默认密钥
		secret = "corex-default-jwt-secret-key-for-development-only"
		fmt.Println("警告: 使用默认JWT密钥，生产环境请设置CORE_X_JWT_SECRET环境变量")
	}

	SetJWTSecret([]byte(secret))

	// 启动黑名单清理定时任务
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			CleanupExpired()
		}
	}()

	fmt.Println("认证系统初始化完成")
	return nil
}

// Cleanup 清理认证系统资源
func Cleanup() {
	// TODO: 实现认证系统清理逻辑
	fmt.Println("认证系统清理完成")
}
