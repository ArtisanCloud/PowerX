package handlers

import (
	"context"
	"github.com/ArtisanCloud/PowerX/services/mcp/register"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
)

// ToolsHandler 工具处理器
type ToolsHandler struct {
	registry *register.ToolRegistry
}

// NewToolsHandler 创建工具处理器
func NewToolsHandler(registry *register.ToolRegistry) *ToolsHandler {
	return &ToolsHandler{
		registry: registry,
	}
}

// HandleListTools 列出所有工具 - 动态从注册表获取
func (h *ToolsHandler) HandleListTools(c *gin.Context) {
	// 检查是否请求简化版本
	simple := c.Query("simple") == "true"

	// 获取所有工具规范（包含完整信息）
	allToolSpecs := h.registry.GetAllToolSpecsTyped()

	// 如果有工具规范，可以返回完整的ToolSpec对象或简化版本
	if len(allToolSpecs) > 0 {
		if simple {
			// 返回简化版本
			tools := make([]gin.H, 0, len(allToolSpecs))
			for toolId, spec := range allToolSpecs {

				tools = append(tools, gin.H{
					"id":          toolId,
					"name":        spec.Name,
					"description": spec.Description,
					"version":     spec.Version,
				})

			}
			c.JSON(http.StatusOK, gin.H{
				"tools":  tools,
				"count":  len(tools),
				"source": "tool_specs_simple",
			})
		} else {
			// 返回完整的ToolSpec对象
			tools := make([]interface{}, 0, len(allToolSpecs))
			for _, spec := range allToolSpecs {
				tools = append(tools, spec)
			}
			c.JSON(http.StatusOK, gin.H{
				"tools":  tools,
				"count":  len(tools),
				"source": "tool_specs_full",
			})
		}
		return
	}

	// 兼容模式：如果没有工具规范，使用原有方式
	allTools := h.registry.GetAllToolSpecsTyped()
	tools := make([]gin.H, 0, len(allTools))
	for toolId := range allTools {
		tools = append(tools, gin.H{
			"id":          toolId,
			"name":        toolId,
			"description": h.registry.GetToolDescription(toolId),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"tools":  tools,
		"count":  len(tools),
		"source": "handlers_fallback",
	})
}

// HandleCallTool 调用指定工具 - 动态从注册表调用
func (h *ToolsHandler) HandleCallTool(c *gin.Context) {
	toolId := c.Param("toolId")

	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// 从注册表获取工具处理器
	handler, exists := h.registry.GetToolHandler(toolId)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tool not found"})
		return
	}

	// 创建 MCP 请求
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolId,
			Arguments: params,
		},
	}

	// 调用工具处理器
	result, err := handler(context.Background(), request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
