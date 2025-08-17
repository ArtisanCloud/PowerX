package main

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/http"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/ArtisanCloud/PowerX/services/agent/bootstrap"
	"log"
)

func main() {
	ctx := context.Background()

	// 1. 加载统一配置
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		log.Fatalf("加载配置文件失败")
	}

	// 2. 初始化全局Logger
	// 使用配置中的日志配置初始化全局Logger
	logger.InitGlobalLogger(&cfg.LogConfig)
	// 测试全局Logger是否工作正常
	logger.Info(ctx, "🚀 全局Logger初始化成功")

	// 4. 初始化工具（agent_tools）
	err := bootstrap.InitAgentTools(ctx, &cfg.Agent)
	if err != nil {
		logger.ErrorF(ctx, "初始化工具失败: %s", err.Error())
		return
	}

	// 5. 初始化事件总线 / 插件订阅
	err = event_bus.InitEventBus()
	if err != nil {
		logger.ErrorF(ctx, "初始化事件总线失败: %s", err.Error())
	}

	// 6. 构建 router 并挂载路由
	r := http.SetupRouter(cfg)

	// 7. 打印路由信息
	//httpRouter.PrintRouteInfo(r, cfg)

	// 8. 启动服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.InfoF(ctx, "🚀 CoreX 服务启动成功！监听地址: http://localhost%s\n", addr)
	err = r.Run(addr)
	if err != nil {
		logger.ErrorF(ctx, "启动服务失败: %s", err.Error())
	}
}
