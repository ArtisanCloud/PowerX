package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	httpAdmin "github.com/ArtisanCloud/PowerX/internal/transport/http/admin"
	systemHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/system"
	httpOpenAPI "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi"
	"github.com/ArtisanCloud/PowerX/internal/transport/websocket"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
)

// SetupRouter 构造带基础中间件的 Gin 引擎，外部传入 auth middleware 和自定义 route 注册函数。
// registerFunc 会在 corexGroup 上执行（即 /{prefix}/... 下面），返回 engine 供外部再挂载其他 group/handler。
func SetupRouter(cfg *config.Config, r *gin.Engine, deps *shared.Deps) error {
	registerAuthSubjectCacheInvalidation(deps.DB)

	// 全局中间件：恢复/日志/trace/feature 等
	r.Use(RecoveryMiddleware())
	r.Use(RequestLoggingMiddleware())
	r.Use(TraceInjectionMiddleware())
	r.Use(FeatureInjectionMiddleware())
	r.Use(InstallGuardMiddleware(cfg))

	authUser := middleware.APIKeyOrJwtMiddleware(
		deps.DB,
		[]byte(cfg.Auth.JWTSecret),
		cfg.Auth.Issuer,
		[]string{cfg.Auth.AudienceUser},
		[]string{"access"},
		buildJWTSubjectValidationCallback(deps.DB),
		middleware.WithTenantHeaderPolicy(middleware.TenantHeaderPolicy{
			RequireUUID: cfg.Tenants.RequireUUID,
		}),
		middleware.WithExtraTokenChecks(middleware.TokenCheck{
			Issuer:    "powerx-sts",
			Audiences: []string{"powerx:api"},
		}),
	)
	// 给外部注册 CoreX Admin 相关路由（含 ops 管理域预留挂载点）
	httpAdmin.RegisterAPIRoutes(r, authUser, cfg, deps)
	// 给外部注册开放平台（Tenant/OpenAPI）路由
	httpOpenAPI.RegisterAPIRoutes(r, authUser, cfg, deps)

	// 给外部注册 Web 相关 routes（web 端）
	//authUserMW := auth.JwtMiddleware(
	//	cfg.Auth.Issuer,
	//	[]string{cfg.Auth.AudienceUser},
	//	[]string{"access"},
	//	nil,
	//)
	//httpWeb.RegisterAPIRoutes(r, authUserMW, cfg)
	//httpMP.RegisterAPIRoutes(r, authAdminMiddleware, cfg)

	websocket.RegisterWSRoutes(r, authUser, cfg, deps)

	return nil
}

// SetupSetupOnlyRouter 在安装模式下仅挂载 setup/health 相关路由，避免 DB/Cache 强依赖阻断首装。
func SetupSetupOnlyRouter(cfg *config.Config, r *gin.Engine) error {
	if cfg == nil || r == nil {
		return fmt.Errorf("setup-only 路由初始化失败: cfg 或 router 为空")
	}
	r.Use(RecoveryMiddleware())
	r.Use(RequestLoggingMiddleware())
	r.Use(TraceInjectionMiddleware())
	r.Use(FeatureInjectionMiddleware())
	r.Use(InstallGuardMiddleware(cfg))

	prefix := cfg.Server.APIPrefix
	if prefix == "" {
		prefix = "/api"
	}
	r.GET("/healthz", httpAdmin.HealthHandler)
	publicGroup := r.Group(prefix)
	publicGroup.GET("/health", httpAdmin.HealthHandler)

	hSetup := systemHTTP.NewSetupHandler(nil)
	publicGroup.GET("/admin/setup/status", hSetup.Status)
	publicGroup.GET("/admin/setup/config", hSetup.GetConfig)
	publicGroup.PUT("/admin/setup/config", hSetup.SaveConfig)
	publicGroup.POST("/admin/setup/provision", hSetup.Provision)
	publicGroup.POST("/admin/setup/complete", hSetup.Complete)
	publicGroup.POST("/admin/setup/test/database", hSetup.TestDatabaseConnection)
	publicGroup.POST("/admin/setup/test/cache", hSetup.TestCacheConnection)
	publicGroup.POST("/admin/setup/llm/test-connection", hSetup.TestLLMConnection)
	publicGroup.POST("/admin/setup/llm/test-call", hSetup.TestLLMQuickCall)

	return nil
}

// PrintRouteInfo 打印路由信息
func PrintRouteInfo(r *gin.Engine, cfg *config.Config) {
	ctx := logger.WithLogFields(context.Background(), map[string]interface{}{"module": "http.router"})
	logger.Info(ctx, "")
	logger.Info(ctx, "📋 已注册的 API 路由（摘要）:")
	logger.Info(ctx, "┌─────────────────────────────────────────────────────────────┐")
	logger.InfoF(ctx, "│ 服务地址: http://localhost:%-29d│", cfg.Server.Port)
	logger.Info(ctx, "├─────────────────────────────────────────────────────────────┤")
	routes := r.Routes()
	if len(routes) == 0 {
		logger.Info(ctx, "│ (未发现已注册路由，请确认路由是否在 bootstrap 阶段同步注册)        │")
	}

	for _, route := range routes {
		handlerName := trimHandlerName(route.Handler)
		logger.InfoF(ctx, "│ %-6s %-25s - %-60s │", route.Method, route.Path, handlerName)
	}
	logger.Info(ctx, "└─────────────────────────────────────────────────────────────┘")
	logger.Info(ctx, "")
}

// trimHandlerName 提取 handler 名字的最后一段
func trimHandlerName(fullHandler string) string {
	if fullHandler == "" {
		return ""
	}
	parts := strings.Split(fullHandler, ".")
	return parts[len(parts)-1]
}
