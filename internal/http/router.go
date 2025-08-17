package http

import (
	"fmt"
	httpAdmin "github.com/ArtisanCloud/PowerX/api/http/admin"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"strings"

	"github.com/gin-gonic/gin"
)

// initAuth 设置全局 JWT secret 并返回 gin middleware 实例
func initAuth(cfg *config.Config, expectedAudience string, requiredScopes []string) gin.HandlerFunc {
	// 赋值给 auth 包
	auth.SetJWTSecret([]byte(cfg.Auth.JWTSecret))

	// 传入 SampleCallback 做扩展判断&事件广播
	return auth.JwtMiddleware(expectedAudience, requiredScopes, auth.SampleCallback)
}

// SetupRouter 构造带基础中间件的 Gin 引擎，外部传入 auth middleware 和自定义 route 注册函数。
// registerFunc 会在 corexGroup 上执行（即 /{prefix}/... 下面），返回 engine 供外部再挂载其他 group/handler。
func SetupRouter(cfg *config.Config) *gin.Engine {

	r := gin.New()

	// 全局中间件：恢复/日志/trace/feature 等
	r.Use(RecoveryMiddleware())
	r.Use(RequestLoggingMiddleware())
	r.Use(TraceInjectionMiddleware())
	r.Use(FeatureInjectionMiddleware())

	// 给外部注册 CoreX 相关 routes（discovery / sample orchestrator/tool 等）
	authAdminMiddleware := initAuth(cfg, "user", []string{})
	httpAdmin.RegisterAPIRoutes(r, authAdminMiddleware, cfg)

	// 给外部注册 Web 相关 routes（web 端）
	// authCustomerMiddleware := initAuth(cfg, "customer", []string{"*"})
	//httpWeb.RegisterAPIRoutes(r, authAdminMiddleware, cfg)
	//httpMP.RegisterAPIRoutes(r, authAdminMiddleware, cfg)

	return r
}

// PrintRouteInfo 打印路由信息
func PrintRouteInfo(r *gin.Engine, cfg *config.Config) {
	fmt.Println()
	fmt.Println("📋 已注册的 API 路由（摘要）:")
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Printf("│ 服务地址: http://localhost:%-29d│\n", cfg.Server.Port)
	fmt.Println("├─────────────────────────────────────────────────────────────┤")
	routes := r.Routes()
	if len(routes) == 0 {
		fmt.Println("│ (未发现已注册路由，请确认路由是否在 bootstrap 阶段同步注册)        │")
	}

	for _, route := range routes {
		handlerName := trimHandlerName(route.Handler)
		fmt.Printf("│ %-6s %-25s - %-60s │\n", route.Method, route.Path, handlerName)
	}
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Println()
}

// trimHandlerName 提取 handler 名字的最后一段
func trimHandlerName(fullHandler string) string {
	if fullHandler == "" {
		return ""
	}
	parts := strings.Split(fullHandler, ".")
	return parts[len(parts)-1]
}
