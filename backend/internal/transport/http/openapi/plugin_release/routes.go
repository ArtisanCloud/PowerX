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

	if handler := newLocalInstallHandler(deps.PluginReleaseService.LocalInstall(), deps.DB); handler != nil {
		routes := group.Group("/tenant/plugin-release")
		routes.POST("/install-sessions", handler.startSession)
		routes.GET("/install-sessions/:session_uuid", handler.getSession)
		routes.POST("/install-sessions/:session_uuid/stop", handler.stopSession)
	}

	if importHandler := newOfflineImportHandler(deps.PluginReleaseService.Distribution(), deps.DB); importHandler != nil {
		importRoutes := group.Group("/tenant/plugin-release/import-jobs")
		importRoutes.POST("", importHandler.startImport)
		importRoutes.GET("/:job_uuid", importHandler.getImport)
	}
}
