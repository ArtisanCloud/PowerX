package agent

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// Register 将 OpenAPI 的代理健康路由注册到受保护的组。
func Register(public *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil || deps.AgentLifecycle == nil || deps.AgentLifecycle.Service == nil {
		return
	}
	handler := NewHandler(deps.AgentLifecycle.Service)
	group := protected.Group("/openapi/agents")
	{
		group.GET("/:agent_id/health/summary", handler.GetHealthSummary)
		group.GET("/:agent_id/health/history", handler.ListHealthHistory)
	}
}
