package capability_registry

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterTenantRoutes wires tenant-facing capability registry endpoints under /tenant.
func RegisterTenantRoutes(group *gin.RouterGroup, deps *shared.Deps) {
	if group == nil || deps == nil || deps.CapabilityCatalogSvc == nil {
		return
	}
	handler := newTenantHandler(deps)
	if handler == nil {
		return
	}
	tenantGroup := group.Group("/tenant")
	{
		tenantGroup.GET("/capabilities", handler.ListCapabilities)
		tenantGroup.POST("/invocations", handler.InvokeCapability)
		tenantGroup.POST("/invocations/stream", handler.InvokeCapabilityStream)
		tenantGroup.GET("/invocations/:traceId", handler.GetInvocation)
	}
}
