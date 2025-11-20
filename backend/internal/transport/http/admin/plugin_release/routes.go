package plugin_release

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
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
	group.Use(middleware.AdminOnlyMiddleware())
	group.POST("/candidates", handler.createCandidate)
	group.GET("/candidates/:candidateId", handler.getCandidate)
	group.POST("/candidates/:candidateId/gates", handler.runGates)
	group.POST("/plans", handler.createPlan)

	if runtimeHandler := newDeploymentHandler(deps.PluginReleaseService.Runtime()); runtimeHandler != nil {
		group.POST("/plans/:planId/deploy/canary", runtimeHandler.triggerCanary)
		group.POST("/plans/:planId/deploy/finalize", runtimeHandler.finalizeDeployment)
		group.POST("/plans/:planId/deploy/rollback", runtimeHandler.rollback)
	}

	if distHandler := newDistributionHandler(deps.PluginReleaseService.Distribution()); distHandler != nil {
		group.POST("/offline-packages", distHandler.createOfflinePackage)
		group.GET("/marketplace/listings", distHandler.getMarketplaceListings)
		group.POST("/marketplace/listings", distHandler.createListing)
		group.POST("/marketplace/listings/:listingId/reviews", distHandler.reviewListing)
	}

	if publishHandler := newPublishHandler(deps.PluginReleaseService); publishHandler != nil {
		registry := protected.Group("/internal/plugins")
		registry.Use(middleware.AdminOnlyMiddleware())
		registry.POST("/releases", publishHandler.createRelease)
		registry.GET("/releases/:candidateId", publishHandler.getRelease)
		registry.PATCH("/releases/:candidateId", publishHandler.updateRelease)
		registry.POST("/releases/:candidateId/artifacts", publishHandler.uploadArtifact)
	}
}
