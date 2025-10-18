package event_fabric

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 注册事件骨干相关 Admin API。
func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil || deps.EventFabric == nil || deps.EventFabric.Directory == nil {
		return
	}

	handler := NewAdminDirectoryHandler(AdminDirectoryHandlerOptions{Service: deps.EventFabric.Directory})
	group := protected.Group("/event-fabric")
	group.POST("/topics", handler.CreateTopic)
	group.GET("/topics", handler.ListTopics)
	group.PATCH("/topics/:topic_id/lifecycle", handler.UpdateLifecycle)
}
