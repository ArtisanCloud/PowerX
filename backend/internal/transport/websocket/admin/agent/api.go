package agent

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterWSRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {

	agentGroup := protectedGroup.Group("/agents")
	wsH := NewAgentWSHandler(deps)
	{
		agentGroup.GET("/stream/ws", wsH.StreamWS) // 新增：WebSocket（事件通知/双向流）
	}

}
