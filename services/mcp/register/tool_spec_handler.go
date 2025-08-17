package register

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/services/mcp/register/factory"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// mcp/register/tool_spec_handler.go

// ToolSpecHandler 定义基于规范的动态处理器接口
type ToolSpecHandler interface {
	CreateDynamicHandler(spec *schemas.ToolSpec) server.ToolHandlerFunc
	GetHandlerInfo(toolID string) map[string]interface{}
	ListAvailableHandlers() map[string]interface{}
}

// 全局规范处理器单例
var globalToolSpecHandler ToolSpecHandler

// GetToolSpecHandler 获取或初始化全局 ToolSpecHandler
func GetToolSpecHandler() ToolSpecHandler {
	if globalToolSpecHandler == nil {
		globalToolSpecHandler = NewToolSpecHandler(globalRegistry)
	}
	return globalToolSpecHandler
}

// NewToolSpecHandler 创建 ToolSpecHandler，实际实现位于 handlers 包
func NewToolSpecHandler(registry *ToolRegistry) ToolSpecHandler {
	return &defaultToolSpecHandler{registry: registry}
}

// defaultToolSpecHandler 默认实现
type defaultToolSpecHandler struct {
	registry *ToolRegistry
}

func (h *defaultToolSpecHandler) CreateDynamicHandler(spec *schemas.ToolSpec) server.ToolHandlerFunc {
	hf := factory.NewHandlerFactory() // 1. 拿到工厂
	handler := hf.CreateHandler(spec) // 2. 生成真正的 handler
	if handler == nil {
		// 3. 工厂没给，就退回到一个“未实现”的占位
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("❌ 工具 %s 的 HandlerType=%q 尚未实现", spec.ID, spec.HandlerType),
					},
				},
			}, nil
		}
	}
	return handler
}

func (h *defaultToolSpecHandler) GetHandlerInfo(toolID string) map[string]interface{} {
	spec, ok := h.registry.GetToolSpec(toolID)
	if !ok {
		return map[string]interface{}{"error": "工具规范不存在"}
	}
	return map[string]interface{}{
		"tool_id": spec.ID,
		"name":    spec.Name,
		"version": spec.Version,
	}
}

func (h *defaultToolSpecHandler) ListAvailableHandlers() map[string]interface{} {
	all := h.registry.GetAllToolSpecsTyped()
	handlers := make(map[string]interface{})
	for id, spec := range all {
		handlers[id] = map[string]interface{}{
			"name":    spec.Name,
			"version": spec.Version,
		}
	}
	return map[string]interface{}{
		"count":    len(handlers),
		"handlers": handlers,
	}
}
