package handlers

import (
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
type HealthHandler struct {
	config *config.MCPConfig
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(cfg *config.MCPConfig) *HealthHandler {
	return &HealthHandler{
		config: cfg,
	}
}

// Handle 处理健康检查请求
func (h *HealthHandler) Handle(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "CoreX-MCP",
		"version": "v0.1.0",
		"port":    h.config.Server.Port,
	})
}
