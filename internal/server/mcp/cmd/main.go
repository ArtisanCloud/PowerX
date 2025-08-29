package main

import (
	"context"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"log"
)

// mcp/cmd/main.go

func main() {
	// 加载配置
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		log.Fatalf("加载配置文件失败")
	}

	ctx := context.Background()
	// 使用配置中的日志配置初始化全局Logger
	logger.InitGlobalLogger(&cfg.LogConfig)
	// 测试全局Logger是否工作正常
	logger.Info(ctx, "🚀 全局Logger初始化成功")

	// 创建并启动服务器
	server := mcp.NewServer(cfg)

	// 启动服务器
	if err := server.Start(ctx); err != nil {
		logger.InfoF(ctx, "启动服务器失败: %v", err)
	} else {
		logger.Info(ctx, "🚀 服务器启动成功")
	}
}
