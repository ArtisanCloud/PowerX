package plugin_release

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes wires admin-side plugin release HTTP endpoints (placeholder until handlers exist).
func RegisterAPIRoutes(public, protected *gin.RouterGroup, deps *shared.Deps) {
	_ = deps
	_ = public
	if protected == nil {
		return
	}
	_ = protected
	// TODO: add admin handlers for plugin release lifecycle in later phases.
}
