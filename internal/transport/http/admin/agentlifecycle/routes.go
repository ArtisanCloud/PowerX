package agentlifecycle

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// Register 将生命周期相关路由挂载到管理员组。
func Register(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	h := NewHandler(deps)

	group := protectedGroup.Group("/admin/agent/lifecycle")
	{
		group.POST("/agents", h.RegisterAgent)
		group.GET("/agents", h.ListAgents)
		group.GET("/agents/:agent_id", h.GetAgent)
		group.POST("/agents/:agent_id/activate", h.ActivateAgent)
	}
}
