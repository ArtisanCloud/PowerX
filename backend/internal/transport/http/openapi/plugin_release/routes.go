package plugin_release

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterTenantRoutes wires tenant-facing plugin release HTTP endpoints (placeholder).
func RegisterTenantRoutes(group *gin.RouterGroup, deps *shared.Deps) {
	_ = deps
	if group == nil {
		return
	}
	// TODO: add offline import / rollout tenant APIs in later phases.
}
