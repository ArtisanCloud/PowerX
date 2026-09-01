package notifications

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterTenantRoutes registers tenant notification creation routes.
func RegisterTenantRoutes(protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil {
		return
	}
	h := newHandler(deps)
	group := protectedGroup.Group("/notifications")
	group.POST("", h.create)
}
