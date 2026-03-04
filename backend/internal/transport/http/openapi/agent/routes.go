package agent

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// Register 将 OpenAPI 的代理健康路由注册到受保护的组。
func Register(public *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil {
		return
	}
	if deps.AgentLifecycle != nil && deps.AgentLifecycle.Service != nil {
		handler := NewHandler(deps.AgentLifecycle.Service)
		group := protected.Group("/openapi/agents")
		{
			group.GET("/:agent_id/health/summary", handler.GetHealthSummary)
			group.GET("/:agent_id/health/history", handler.ListHealthHistory)
			group.GET("/:agent_id/bridge/state", handler.GetBridgeState)
			group.POST("/:agent_id/bridge/freeze", handler.FreezeAgent)
			group.POST("/:agent_id/bridge/recover", handler.RecoverAgent)
			group.POST("/:agent_id/bridge/rebalance", handler.RebalanceAgent)
		}
	}
}
