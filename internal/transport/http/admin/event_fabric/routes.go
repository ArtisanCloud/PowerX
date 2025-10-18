package event_fabric

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 注册事件骨干相关 Admin API。
func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil || deps.EventFabric == nil {
		return
	}

	group := protected.Group("/event-fabric")

	if deps.EventFabric.Directory != nil {
		dirHandler := NewAdminDirectoryHandler(AdminDirectoryHandlerOptions{Service: deps.EventFabric.Directory})
		group.POST("/topics", dirHandler.CreateTopic)
		group.GET("/topics", dirHandler.ListTopics)
		group.PATCH("/topics/:topic_id/lifecycle", dirHandler.UpdateLifecycle)
	}

    if deps.EventFabric.ACL != nil && deps.EventFabric.Directory != nil {
        aclHandler := NewAdminACLHandler(AdminACLHandlerOptions{
            Service:   deps.EventFabric.ACL,
            Directory: deps.EventFabric.Directory,
        })
        group.POST("/acl", aclHandler.UpsertBindings)
        group.GET("/acl", aclHandler.ListBindings)
    }
}
