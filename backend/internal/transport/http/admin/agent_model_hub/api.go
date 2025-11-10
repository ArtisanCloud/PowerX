package agentmodelhub

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes exposes Agent Model Hub endpoints under /api/internal.
func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil || deps == nil || deps.DB == nil {
		return
	}

	providerHandler := NewProviderHandler(deps)
	routingHandler := NewRoutingHandler(deps)
	internalGroup := protectedGroup.Group("/internal")
	{
		internalGroup.POST("/providers/register", providerHandler.registerProvider)
		internalGroup.POST("/providers/:providerId/validate", providerHandler.validateProvider)
		internalGroup.POST("/providers/:providerId/publish", providerHandler.publishProvider)
		internalGroup.POST("/providers/:providerId/rollout", providerHandler.rolloutProvider)
		internalGroup.POST("/providers/:providerId/rollback", providerHandler.rollbackProvider)

		internalGroup.POST("/model-routing/policies", routingHandler.publishPolicy)
		internalGroup.POST("/model-routing/policies/status", routingHandler.updatePolicyStatus)
		internalGroup.POST("/model-routing/route", routingHandler.routeTask)
		internalGroup.POST("/model-routing/rollback", routingHandler.rollbackPolicy)
		internalGroup.POST("/model-routing/safe-mode", routingHandler.toggleSafeMode)
	}
}
