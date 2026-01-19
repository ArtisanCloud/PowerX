package notifications

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	h := NewHandler(deps)
	adminGroup := protectedGroup.Group("/admin/notifications")
	adminGroup.POST("/test", h.PushTestNotification)
}
