package main

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	"github.com/ArtisanCloud/PowerX/internal/http"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"log"
)

func main() {
	ctx := context.Background()

	r := gin.New()

	// 1. 加载统一配置
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		log.Fatalf("加载配置文件失败")
	}

	// bootstrap app
	err := bootstrap.BootstrapApp(ctx, cfg)
	if err != nil {
		logger.ErrorF(ctx, "BootstrapApp failed: %s", err.Error())
		return
	}

	// bootstrap plugin manager
	_, err = bootstrap.BootstrapPlugin(ctx, cfg, r)
	if err != nil {
		logger.ErrorF(ctx, "BootstrapPlugin failed: %s", err.Error())
		return
	}
	//// 临时：启用一个插件
	//if err := pluginMgr.Enable(ctx, "com.powerx.demo.hello_world"); err != nil {
	//	logger.ErrorF(ctx, "enable plugin failed: %v", err)
	//}

	// 6. 构建 router 并挂载路由
	err = http.SetupRouter(cfg, r)
	if err != nil {
		logger.ErrorF(ctx, "SetupRouter failed: %s", err.Error())
		return
	}

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
