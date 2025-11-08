package handlers

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/config"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/register"

	"github.com/gin-gonic/gin"
)

// MCPHandler MCP协议处理器
type MCPHandler struct {
	config   *config.MCPConfig
	registry *register.ToolRegistry
}

// NewMCPHandler 创建MCP协议处理器
func NewMCPHandler(cfg *config.MCPConfig, registry *register.ToolRegistry) *MCPHandler {
	return &MCPHandler{
		config:   cfg,
		registry: registry,
	}
}

// HandleSSE 处理 SSE 连接
func (h *MCPHandler) HandleSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 发送连接成功消息
	c.SSEvent("message", map[string]interface{}{
		"type":   "connection",
		"status": "connected",
		"server": "CoreX MCP v0.1.0",
	})

	c.Writer.Flush()
}

// HandleMCPMessage 处理 MCP 协议消息
func (h *MCPHandler) HandleMCPMessage(c *gin.Context) {
	var request map[string]interface{}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(400, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      nil,
			"error": map[string]interface{}{
				"code":    -32700,
				"message": "Parse error",
			},
		})
		return
	}

	// 检查基本的 JSON-RPC 格式
	method, ok := request["method"].(string)
	if !ok {
		c.JSON(400, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"error": map[string]interface{}{
				"code":    -32600,
				"message": "Invalid Request",
			},
		})
		return
	}

	// 处理不同的方法
	switch method {
	case "tools/list":
		h.handleToolsList(c, request)
	case "tools/call":
		h.handleToolsCall(c, request)
	default:
		c.JSON(400, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"error": map[string]interface{}{
				"code":    -32601,
				"message": "Method not found",
			},
		})
	}
}

// handleToolsList 处理工具列表请求
func (h *MCPHandler) handleToolsList(c *gin.Context, request map[string]interface{}) {
	// 检查 sessionId 参数
	params, ok := request["params"].(map[string]interface{})
	if !ok {
		c.JSON(400, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "Invalid params",
			},
		})
		return
	}

	// 验证 sessionId 是否存在
	if _, exists := params["sessionId"]; !exists {
		c.JSON(400, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "Missing sessionId",
			},
		})
		return
	}

	allTools := h.registry.GetAllToolSpecsTyped()
	tools := []map[string]interface{}{}

	for toolId := range allTools {
		tools = append(tools, map[string]interface{}{
			"name":        toolId,
			"description": h.registry.GetToolDescription(toolId),
		})
	}

	c.JSON(200, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      request["id"],
		"result": map[string]interface{}{
			"tools": tools,
		},
	})
}

// handleToolsCall 处理工具调用请求
func (h *MCPHandler) handleToolsCall(c *gin.Context, request map[string]interface{}) {
	params, ok := request["params"].(map[string]interface{})
	if !ok {
		c.JSON(400, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "Invalid params",
			},
		})
		return
	}

	// 验证 sessionId 是否存在
	if _, exists := params["sessionId"]; !exists {
		c.JSON(400, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "Missing sessionId",
			},
		})
		return
	}

	toolName, ok := params["name"].(string)
	if !ok {
		c.JSON(400, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "Missing tool name",
			},
		})
		return
	}

	// 从注册表获取工具处理器
	_, exists := h.registry.GetToolSpec(toolName)
	if !exists {
		c.JSON(404, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      request["id"],
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "Tool not found",
			},
		})
		return
	}

	// 构造工具调用请求（这里简化处理）
	arguments, _ := params["arguments"].(map[string]interface{})

	// 调用工具（这里需要适配 mark3labs 的接口）
	result := map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("工具 %s 执行成功，参数: %v", toolName, arguments),
			},
		},
	}

	c.JSON(200, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      request["id"],
		"result":  result,
	})
}
