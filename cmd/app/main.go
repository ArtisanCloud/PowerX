package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	"github.com/ArtisanCloud/PowerX/internal/http"
	"github.com/ArtisanCloud/PowerX/internal/openapi"
	grpcserver "github.com/ArtisanCloud/PowerX/internal/server/grpc"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	docs "github.com/ArtisanCloud/PowerX/docs"
)

// @title       PowerX Admin API
// @version     v1.0.0
// @description PowerX 核心与插件管理 API
// @BasePath    /
func main() {
	ctx := context.Background()

	r := gin.New()

	// Swagger 文档元信息（也可在 docs/docs.go 中修改默认生成的内容）
	docs.SwaggerInfo.Title = "PowerX Admin API"
	docs.SwaggerInfo.Description = "PowerX 核心与插件管理 API"
	docs.SwaggerInfo.Version = "v1.0.0"
	docs.SwaggerInfo.BasePath = "/"

	// 加载全局配置
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		log.Fatalf("加载配置文件失败")
	}

	// 初始化应用核心依赖
	deps, err := bootstrap.BootstrapApp(ctx, cfg)
	if err != nil {
		logger.ErrorF(ctx, "BootstrapApp failed: %s", err.Error())
		return
	}
	defer deps.AuditSvc.Close()
	r.Use(gin.Recovery())
	r.Use(audit.GinAudit(deps.Auditor))

	// 初始化插件管理器
	_, err = bootstrap.BootstrapPlugin(ctx, deps, cfg, r)
	if err != nil {
		logger.ErrorF(ctx, "BootstrapPlugin failed: %s", err.Error())
		return
	}

	// 配置 HTTP 路由
	err = http.SetupRouter(cfg, r, deps)
	if err != nil {
		logger.ErrorF(ctx, "SetupRouter failed: %s", err.Error())
		return
	}

	// 启动 gRPC 服务
	_, err = grpcserver.BootstrapGRPC(ctx, &cfg.Server.GRPC, deps)
	if err != nil {
		logger.ErrorF(ctx, "BootstrapGRPC failed: %s", err.Error())
		return
	}

	// 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.InfoF(ctx, "🚀 CoreX 服务启动成功！监听地址: http://localhost%s\n", addr)

	// 挂载 Swagger UI（UI 默认读取 swagger.json，也可指定 openapi.min.json）
	r.GET("/swagger/*any",
		ginSwagger.WrapHandler(
			swaggerFiles.Handler,
			ginSwagger.URL("/openapi.min.json"), // 指定 Swagger UI 使用的 JSON
		),
	)

	// 挂载最小化 OpenAPI 文档（轻量模式）
	openapi.MountMinimalOpenAPI(r, openapi.Info{
		Title: "PowerX Admin API (Minimal)", Version: "v1.0.0",
	})

	// 生成并保存最小 OpenAPI 文档文件
	if err := openapi.SaveMinimalDoc(r, openapi.Info{
		Title:   "PowerX Admin API (Minimal)",
		Version: "v1.0.0",
		BaseURL: "/",
	}, "./docs"); err != nil {
		logger.ErrorF(ctx, "写入最小 OpenAPI 文档失败: %s", err.Error())
	}

	// 运行 HTTP 服务
	err = r.Run(addr)
	if err != nil {
		logger.ErrorF(ctx, "启动服务失败: %s", err.Error())
	}

}
