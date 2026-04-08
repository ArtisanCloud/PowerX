package skills

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterTenantRoutes wires tenant-facing skill invoke routes.
func RegisterTenantRoutes(group *gin.RouterGroup, deps *shared.Deps) {
	if group == nil || deps == nil || deps.DB == nil {
		return
	}
	handler := newTenantHandler(deps)
	if handler == nil {
		return
	}
	tenantGroup := group.Group("/tenant")
	tenantGroup.POST("/skills/invoke", handler.InvokeDirect)
}
