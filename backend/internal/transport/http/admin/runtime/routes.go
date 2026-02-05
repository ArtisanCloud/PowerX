package runtime

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil {
		return
	}
	h := newWSBusHandler()
	internalGroup := protectedGroup.Group("/internal")
	internalGroup.POST("/ws-bus/publish", h.publish)
}
