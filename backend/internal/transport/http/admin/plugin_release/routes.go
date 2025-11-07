package plugin_release

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes wires admin-side plugin release HTTP endpoints.
func RegisterAPIRoutes(public, protected *gin.RouterGroup, deps *shared.Deps) {
	_ = public
	if protected == nil || deps == nil || deps.PluginReleaseService == nil {
		return
	}
	handler := newReleaseGuardrailHandler(deps.PluginReleaseService.Pipeline())
	if handler == nil {
		return
	}
	group := protected.Group("/plugin-release")
	group.POST("/candidates", handler.createCandidate)
	group.GET("/candidates/:candidateId", handler.getCandidate)
	group.POST("/candidates/:candidateId/gates", handler.runGates)
	group.POST("/plans", handler.createPlan)
}
