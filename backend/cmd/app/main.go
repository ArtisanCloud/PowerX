package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	"github.com/ArtisanCloud/PowerX/internal/http"
	"github.com/ArtisanCloud/PowerX/internal/openapi"
	agentcatalog "github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	grpcserver "github.com/ArtisanCloud/PowerX/internal/server/grpc"
	authorizationService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	apikeypermissions "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	pxcrypto "github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	docs "github.com/ArtisanCloud/PowerX/api/openapi"
)

// @title       PowerX Admin API
// @version     v1.0.0
// @description PowerX 核心与插件管理 API
// @BasePath    /
func main() {
	ctx := context.Background()

	// 加载全局配置
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		log.Fatalf("加载配置文件失败")
	}
	effectivePorts := config.ResolveEffectivePorts(cfg)
	logger.InfoF(
		ctx,
		"startup config_path=%s install_status=%s effective_ports={backend_port:%d,web_admin_port:%d}",
		config.GetGlobalConfigPath(),
		cfg.Install.EffectiveStatus(),
		effectivePorts.BackendPort,
		effectivePorts.WebAdminPort,
	)
	apikeypermissions.SetIntroducedVersion(cfg.EffectiveSystemVersion())

	// Gin 的 debug 路由打印按 log.http_debug 控制
	if cfg.LogConfig.HttpDebug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// Swagger 文档元信息（也可在 api/openapi/docs.go 中修改默认生成的内容）
	docs.SwaggerInfo.Title = "PowerX Admin API"
	docs.SwaggerInfo.Description = "PowerX 核心与插件管理 API"
	docs.SwaggerInfo.Version = "v1.0.0"
	docs.SwaggerInfo.BasePath = "/"

	// 初始化应用核心依赖
	setupOnlyMode := canFallbackToSetupOnly(cfg)
	var (
		deps *shared.Deps
		err  error
	)
	if setupOnlyMode {
		if err := ensureWrapKeyForSetupOnly(ctx, cfg); err != nil {
			logger.ErrorF(ctx, "setup-only wrap key init failed: %v", err)
			return
		}
		ensureCatalogForSetupOnly(ctx, cfg)
		logger.WarnF(
			ctx,
			"install status=%s, setup-only preflight enabled: skip DB/Cache bootstrap",
			cfg.Install.EffectiveStatus(),
		)
	} else {
		deps, err = bootstrap.BootstrapApp(ctx, cfg)
		if err != nil {
			if canFallbackToSetupOnly(cfg) && errors.Is(err, bootstrap.ErrBootstrapDependencyUnavailable) {
				setupOnlyMode = true
				if err = ensureWrapKeyForSetupOnly(ctx, cfg); err != nil {
					logger.ErrorF(ctx, "setup-only wrap key init failed: %v", err)
					return
				}
				ensureCatalogForSetupOnly(ctx, cfg)
				logger.WarnF(ctx, "BootstrapApp failed, fallback to setup-only mode: %v", err)
			} else {
				logger.ErrorF(ctx, "BootstrapApp failed: %s", err.Error())
				return
			}
		}
	}
	r.Use(gin.Recovery())
	if setupOnlyMode {
		err = http.SetupSetupOnlyRouter(cfg, r)
		if err != nil {
			logger.ErrorF(ctx, "SetupSetupOnlyRouter failed: %s", err.Error())
			return
		}
	} else {
		defer deps.AuditSvc.Close()
		r.Use(audit.GinAudit(deps.Auditor))

		// 初始化插件管理器
		_, err = bootstrap.BootstrapPlugin(ctx, deps, cfg, r)
		if err != nil {
			logger.WarnF(ctx, "BootstrapPlugin failed, continue without plugin runtime: %s", err.Error())
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

		if deps.EventFabric != nil {
			if deps.EventFabric.RetryWorker != nil {
				go deps.EventFabric.RetryWorker.Run(ctx)
			}
			if deps.EventFabric.CronDispatcherWorker != nil {
				go deps.EventFabric.CronDispatcherWorker.Run(ctx)
			}
			if deps.EventFabric.NotificationWorker != nil {
				go deps.EventFabric.NotificationWorker.Run(ctx)
			}
			if deps.EventFabric.Authorization != nil {
				if deps.EventFabric.Authorization.TimeoutTaskWorker != nil {
					go deps.EventFabric.Authorization.TimeoutTaskWorker.Run(ctx)
				}
				if deps.EventFabric.Authorization.Service != nil {
					go func() {
						if err := deps.EventFabric.Authorization.Service.ListenCacheInvalidation(ctx); err != nil && err != authorizationService.ErrOperationUnsupported {
							logger.WarnF(ctx, "authorization cache listener stopped: %v", err)
						}
					}()
				}
			}
		}
	}
	if err := assertGlobalWrapKeyInitialized(); err != nil {
		logger.ErrorF(ctx, "startup aborted: %v", err)
		return
	}

	// 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	if setupOnlyMode {
		logger.WarnF(ctx, "🚧 安装模式启动成功（setup-only），监听地址: http://localhost%s", addr)
	} else {
		logger.InfoF(ctx, "🚀 CoreX 服务启动成功！监听地址: http://localhost%s\n", addr)
	}

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

	// 生成并保存最小 OpenAPI 文档文件（兼容不同启动目录）
	docInfo := openapi.Info{
		Title:   "PowerX Admin API (Minimal)",
		Version: "v1.0.0",
		BaseURL: "/",
	}
	if err := saveMinimalOpenAPIDocs(r, docInfo); err != nil {
		logger.ErrorF(ctx, "写入最小 OpenAPI 文档失败: %s", err.Error())
	}

	// 打印路由（受 log.http_debug 控制）
	// if cfg.LogConfig.HttpDebug {
	// 	http.PrintRouteInfo(r, cfg)
	// }

	// 运行 HTTP 服务
	err = r.Run(addr)
	if err != nil {
		logger.ErrorF(ctx, "启动服务失败: %s", err.Error())
	}
}

func canFallbackToSetupOnly(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	if !cfg.Install.AllowWithoutDB {
		return false
	}
	switch cfg.Install.EffectiveStatus() {
	case "uninstalled", "configuring":
		return true
	default:
		return false
	}
}

func ensureCatalogForSetupOnly(ctx context.Context, cfg *config.Config) {
	if cfg == nil {
		return
	}
	if err := agentcatalog.InitFromAppConfig(cfg.AI.Catalog, nil); err != nil {
		logger.WarnF(ctx, "setup-only catalog init failed: %v", err)
		return
	}
	n := len(agentcatalog.GetGlobalAIRegister().Providers("llm"))
	logger.InfoF(ctx, "setup-only catalog initialized, llm providers=%d", n)
}

func ensureWrapKeyForSetupOnly(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if _, err := cfg.Server.ParseKey(); err != nil {
		return err
	}
	logger.Info(ctx, "setup-only wrap key initialized")
	return nil
}

func assertGlobalWrapKeyInitialized() error {
	if strings.TrimSpace(pxcrypto.GetGlobalKeyB64()) == "" {
		return fmt.Errorf("missing GLOBAL_WRAP_MASTER_KEY: wrap key is not initialized")
	}
	return nil
}

func saveMinimalOpenAPIDocs(r *gin.Engine, info openapi.Info) error {
	cwd, _ := os.Getwd()
	candidates := []string{"./api/openapi", "./backend/api/openapi"}
	ordered := make([]string, 0, len(candidates))
	for _, path := range candidates {
		absPath := path
		if !filepath.IsAbs(path) {
			absPath = filepath.Join(cwd, path)
		}
		if stat, err := os.Stat(absPath); err == nil && stat.IsDir() {
			ordered = append(ordered, absPath)
		}
	}
	for _, path := range candidates {
		absPath := path
		if !filepath.IsAbs(path) {
			absPath = filepath.Join(cwd, path)
		}
		exists := false
		for _, done := range ordered {
			if done == absPath {
				exists = true
				break
			}
		}
		if !exists {
			ordered = append(ordered, absPath)
		}
	}
	for _, absPath := range ordered {
		if err := openapi.SaveMinimalDoc(r, info, absPath); err == nil {
			return nil
		}
	}
	return openapi.SaveMinimalDoc(r, info, ordered[0])
}
