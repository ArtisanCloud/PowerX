package runtime

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil {
		return
	}
	h := newWSBusHandler(deps)
	taskQueueHandler := newTaskQueueHandler(deps)

	// Canonical internal endpoints (existing contract).
	internalGroup := protectedGroup.Group("/internal")
	internalGroup.POST("/ws-bus/grant", h.grant)
	internalGroup.POST("/ws-bus/publish", h.publish)

	// Compatibility endpoints for framework/plugin runtimes using admin/runtime prefix.
	compatGroup := protectedGroup.Group("/admin/runtime/internal")
	compatGroup.POST("/ws-bus/grant", h.grant)
	compatGroup.POST("/ws-bus/publish", h.publish)

	// Standardized endpoints for plugin runtimes.
	standardGroup := protectedGroup.Group("/admin/runtime")
	standardGroup.POST("/ws-bus/grant", h.grant)
	standardGroup.POST("/ws-bus/publish", h.publish)
	if taskQueueHandler != nil {
		standardGroup.POST("/task-queue/enqueue", taskQueueHandler.enqueue)
		standardGroup.POST("/task-queue/dequeue", taskQueueHandler.dequeue)
		standardGroup.POST("/task-queue/ack", taskQueueHandler.ack)
		standardGroup.POST("/task-queue/nack", taskQueueHandler.nack)
		standardGroup.POST("/task-queue/retry", taskQueueHandler.retry)
	}
}
