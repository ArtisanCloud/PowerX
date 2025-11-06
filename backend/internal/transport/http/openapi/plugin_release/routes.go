package plugin_release

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterTenantRoutes wires tenant-facing plugin release HTTP endpoints.
func RegisterTenantRoutes(group *gin.RouterGroup, deps *shared.Deps) {
	if group == nil || deps == nil || deps.PluginReleaseService == nil {
		return
	}

	handler := newLocalInstallHandler(deps.PluginReleaseService.LocalInstall())
	if handler == nil {
		return
	}

	routes := group.Group("/tenant/plugin-release")
	routes.POST("/local/sessions", handler.startSession)
	routes.GET("/local/sessions/:sessionId", handler.getSession)
	routes.DELETE("/local/sessions/:sessionId", handler.stopSession)
}
