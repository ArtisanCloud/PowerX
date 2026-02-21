package notifications

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	h := NewHandler(deps)
	adminGroup := protectedGroup.Group("/admin/notifications")
	adminGroup.GET("", h.List)
	adminGroup.GET("/:uuid", h.Get)
	adminGroup.PATCH("/:uuid/read", h.MarkRead)
	adminGroup.DELETE("/:uuid", h.Delete)
	adminGroup.POST("/test", h.PushTestNotification)
	adminGroup.POST("/test-queue", h.PushTestNotificationQueue)
}
