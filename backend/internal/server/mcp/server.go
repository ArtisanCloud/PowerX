package mcp

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/http"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/register"
	integrationtools "github.com/ArtisanCloud/PowerX/internal/server/mcp/tools/integration_gateway"
	igdeps "github.com/ArtisanCloud/PowerX/internal/server/mcp/tools/integration_gateway/deps"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/mark3labs/mcp-go/server"
)

// mcp/server.go

// Server CoreX MCP服务器
type Server struct {
	config    *config.Config
	mcpServer *server.MCPServer
}

// NewServer 创建新的MCP服务器，直接处理MCP协议
func NewServer(cfg *config.Config) *Server {
	ctx := context.Background()
	logger.Info(ctx, "🔧 正在初始化 CoreX MCP 服务器...")

	// 初始化工具注册器
	_ = register.NewToolRegistry(nil)

	// 从配置文件加载工具规范
	logger.Info(ctx, "📋 加载工具规范配置...")
	if err := register.LoadToolSpecsFromConfig(&cfg.MCP); err != nil {
		logger.InfoF(ctx, "⚠️  加载工具规范失败: %v", err)
	} else {
		logger.Info(ctx, "✅ 工具规范加载成功")
	}

	// 创建 MCP 服务器
	logger.InfoF(ctx, "🚀 创建 MCP 服务器实例...")
	mcpServer := server.NewMCPServer("CoreX", "v0.1.0")

	// 注册集成网关工具（如果依赖已就绪）
	if dep, err := igdeps.Get(); err == nil {
		if err := integrationtools.RegisterToolsWithRegistry(register.GetGlobalRegistry(), dep.ToolDependencies); err != nil {
			logger.InfoF(ctx, "⚠️ 注册集成网关工具失败: %v", err)
		}
	}

	// 使用统一的注册表注册所有工具
	logger.Info(ctx, "🔨 注册工具到 MCP 服务器...")
	register.RegisterToolsToServer(mcpServer)

	// 显示已注册的工具信息
	registry := register.GetGlobalRegistry()
	allSpecs := registry.GetAllToolSpecsTyped()
	logger.InfoF(ctx, "📦 已注册 %d 个工具:", len(allSpecs))
	for toolID := range allSpecs {
		logger.InfoF(ctx, "   - %s", toolID)
	}

	return &Server{
		config:    cfg,
		mcpServer: mcpServer,
	}
}

// Start 启动服务器
func (s *Server) Start(ctx context.Context) error {
	logger.InfoF(ctx, "%s", strings.Repeat("=", 60))
	logger.InfoF(ctx, "🚀 CoreX MCP 服务器启动中...")
	logger.InfoF(ctx, "%s", strings.Repeat("=", 60))

	logger.InfoF(ctx, "📡 服务器信息:")
	logger.InfoF(ctx, "   名称: CoreX MCP Server")
	logger.InfoF(ctx, "   版本: v0.1.0")
	logger.InfoF(ctx, "   协议: MCP (Model Context Protocol)")
	logger.InfoF(ctx, "   通信: stdio")

	logger.InfoF(ctx, "🔧 使用说明:")
	logger.InfoF(ctx, "   1. 本服务器使用 stdio 通信方式")
	logger.InfoF(ctx, "   2. 适用于 Agent 插件、CLI 工具等场景")
	logger.InfoF(ctx, "   3. 通过标准输入输出与客户端进行 JSON-RPC 通信")

	logger.InfoF(ctx, "📋 可用工具:")
	registry := register.GetGlobalRegistry()
	allSpecs := registry.GetAllToolSpecsTyped()
	for toolID, spec := range allSpecs {
		logger.InfoF(ctx, "   - %s: %s (v%s)", toolID, spec.Name, spec.Version)
	}

	logger.InfoF(ctx, "🔌 连接示例:")
	logger.InfoF(ctx, "   # MCP 客户端连接")
	logger.InfoF(ctx, "   $ echo '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}' | %s", os.Args[0])
	logger.InfoF(ctx, "   # 或使用 MCP 客户端库进行连接")

	logger.InfoF(ctx, "%s", strings.Repeat("-", 60))
	logger.InfoF(ctx, "✅ 服务器已启动，等待客户端连接...")
	logger.InfoF(ctx, "%s", strings.Repeat("-", 60))

	// 3. 启动 Streamable HTTP Server（默认 endpoint 是 /mcp）
	if s.config.MCP.Server.LaunchMode == "http" { // HTTP 模式
		// 查找可用端口
		availablePort := http.FindAvailablePort(&s.config.MCP, s.config.MCP.Server.Port)
		if availablePort != s.config.MCP.Server.Port {
			log.Printf("⚠️ 配置端口 %d 已被占用，自动切换到端口 %d", s.config.MCP.Server.Port, availablePort)
		}
		addr := fmt.Sprintf("%s:%d", s.config.MCP.Server.Host, availablePort)
		httpSrv := server.NewStreamableHTTPServer(s.mcpServer)
		logger.InfoF(ctx, "🚀 HTTP MCP server listening on %s/mcp", addr)
		return httpSrv.Start(addr)
	} else {
		// 默认启动 MCP 服务器 (stdio 模式)
		return server.ServeStdio(s.mcpServer)
	}
}

// Stop 停止服务器
func (s *Server) Stop(ctx context.Context) error {
	logger.InfoF(ctx, "🛑 CoreX MCP 服务器正在停止...")
	// MCP stdio 服务器会在主进程结束时自动停止
	return nil
}
