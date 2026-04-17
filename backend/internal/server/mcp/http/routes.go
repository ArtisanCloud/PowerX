package http

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/config"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/handlers"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/register"
	"net"

	"github.com/gin-gonic/gin"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// setupRoutes 设置路由
func SetupRoutes(cfg *config.MCPConfig, ginEngine *gin.Engine) {
	// 创建处理器
	gRegister := register.GetGlobalRegistry()

	healthHandler := handlers.NewHealthHandler(cfg)
	toolsHandler := handlers.NewToolsHandler(gRegister)
	mcpHandler := handlers.NewMCPHandler(cfg, gRegister)

	// 健康检查
	ginEngine.GET("/health", healthHandler.Handle)

	// 工具相关端点
	ginEngine.GET("/tools", toolsHandler.HandleListTools)
	ginEngine.POST("/tools/:toolId", toolsHandler.HandleCallTool)

	// MCP 协议端点 - 直接处理 MCP 消息
	ginEngine.GET(cfg.Endpoints.SSE, mcpHandler.HandleSSE)
	ginEngine.POST(cfg.Endpoints.Message, mcpHandler.HandleMCPMessage)
}

// printRouteInfo 动态打印路由信息（参考 internal/http/router.go）
func PrintRouteInfo(cfg *config.MCPConfig, ginEngine *gin.Engine) {
	// logger.InfoF(context.Background(), "%+v", cfg)
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	pxlog.Info(context.Background(), "📍 已注册的 MCP 路由:")
	pxlog.Info(context.Background(), "┌─────────────────────────────────────────────────────────────┐")
	pxlog.InfoF(context.Background(), "│ 服务地址: http://%-42s│", addr)
	pxlog.Info(context.Background(), "├─────────────────────────────────────────────────────────────┤")

	routes := ginEngine.Routes()
	if len(routes) == 0 {
		pxlog.Info(context.Background(), "│ (未发现已注册路由，请确认路由是否正确注册)                    │")
	}

	// 动态获取路由信息并打印
	for _, route := range routes {
		// 从路由的 HandlerName 或其他属性动态获取描述
		description := getRouteDescription(route, cfg)
		pxlog.InfoF(context.Background(), "│ %-6s %-25s - %-25s│", route.Method, route.Path, description)
	}
	pxlog.Info(context.Background(), "└─────────────────────────────────────────────────────────────┘")
}

// getRouteDescription 动态获取路由描述
func getRouteDescription(route gin.RouteInfo, cfg *config.MCPConfig) string {
	// 根据路由的处理器名称动态生成描述
	handlerName := route.Handler

	// 根据路径和方法动态判断功能
	switch {
	case route.Path == "/health" && route.Method == "GET":
		return "服务器健康状态检查"
	case route.Path == "/tools" && route.Method == "GET":
		return "获取所有可用工具"
	case route.Path == "/tools/:toolId" && route.Method == "POST":
		return "调用指定工具执行任务"
	case route.Path == cfg.Endpoints.SSE && route.Method == "GET":
		return "服务器发送事件连接"
	case route.Path == cfg.Endpoints.Message && route.Method == "POST":
		return "MCP 协议消息处理"
	default:
		// 对于未知路由，尝试从处理器名称推断
		if handlerName != "" {
			return fmt.Sprintf("处理器: %s", handlerName)
		}
		return "MCP 服务端点"
	}
}

// findAvailablePort 查找可用端口，从指定端口开始递增
func FindAvailablePort(cfg *config.MCPConfig, startPort int) int {
	port := startPort
	for {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			// 端口可用，关闭监听器并返回端口号
			listener.Close()
			return port
		}
		// 端口被占用，尝试下一个端口
		port++
		// 防止无限循环，限制端口范围
		if port > startPort+100 {
			pxlog.WarnF(context.Background(), "⚠️ 无法在端口范围 %d-%d 内找到可用端口，使用原始端口 %d", startPort, port-1, startPort)
			return startPort
		}
	}
}
