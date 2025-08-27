package main

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	"github.com/ArtisanCloud/PowerX/internal/http"
	"github.com/ArtisanCloud/PowerX/internal/openapi"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"log"

	docs "github.com/ArtisanCloud/PowerX/docs"
)

// @title       PowerX Admin API
// @version     v1.0.0
// @description PowerX 核心与插件管理 API
// @BasePath    /
func main() {
	ctx := context.Background()

	r := gin.New()

	// Swagger 基本信息（也可在 docs/docs.go 内默认生成）
	docs.SwaggerInfo.Title = "PowerX Admin API"
	docs.SwaggerInfo.Description = "PowerX 核心与插件管理 API"
	docs.SwaggerInfo.Version = "v1.0.0"
	docs.SwaggerInfo.BasePath = "/"

	// 1. 加载统一配置
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		log.Fatalf("加载配置文件失败")
	}

	// bootstrap app
	deps, err := bootstrap.BootstrapApp(ctx, cfg)
	if err != nil {
		logger.ErrorF(ctx, "BootstrapApp failed: %s", err.Error())
		return
	}
	defer deps.AuditSvc.Close()
	r.Use(gin.Recovery())
	r.Use(audit.GinAudit(deps.Auditor))

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
	err = http.SetupRouter(cfg, r, deps)
	if err != nil {
		logger.ErrorF(ctx, "SetupRouter failed: %s", err.Error())
		return
	}

	// 7. 打印路由信息
	//httpRouter.PrintRouteInfo(r, cfg)

	// 8. 启动服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.InfoF(ctx, "🚀 CoreX 服务启动成功！监听地址: http://localhost%s\n", addr)

	// UI & JSON
	// swagger.json 可通过：/swagger/doc.json 访问
	//r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/swagger/*any",
		ginSwagger.WrapHandler(
			swaggerFiles.Handler,
			ginSwagger.URL("/openapi.min.json"), // ✅ 告诉 UI 用这个 JSON
		),
	)

	// 最小 OpenAPI 文档
	openapi.MountMinimalOpenAPI(r, openapi.Info{
		Title: "PowerX Admin API (Minimal)", Version: "v1.0.0",
	})

	if err := openapi.SaveMinimalDoc(r, openapi.Info{
		Title:   "PowerX Admin API (Minimal)",
		Version: "v1.0.0",
		BaseURL: "/", // 可按需设置
	}, "./docs"); err != nil {
		logger.ErrorF(ctx, "写入最小 OpenAPI 文档失败: %s", err.Error())
	}

	err = r.Run(addr)
	if err != nil {
		logger.ErrorF(ctx, "启动服务失败: %s", err.Error())
	}
}
