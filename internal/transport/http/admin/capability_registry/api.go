package capability_registry

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 将能力注册管理接口挂载到受保护路由。
func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil {
		return
	}

	if publicGroup != nil && deps.DiscoverySvc != nil {
		discoveryHandler := NewDiscoveryHandler(deps.DiscoverySvc)
		publicGroup.GET("/discovery/:tenantId/:capabilityId", discoveryHandler.GetSnapshot)
		publicGroup.POST("/discovery/sync", discoveryHandler.Sync)
	}

	if protectedGroup == nil || deps.CapabilityRegistrySvc == nil {
		return
	}
	handler := NewAdminHandler(AdminHandlerOptions{
		Service: deps.CapabilityRegistrySvc,
	})
	capabilities := protectedGroup.Group("/admin/capabilities")
	{
		capabilities.POST("", handler.CreateCapability)
	}
	tenantScoped := capabilities.Group("/:capabilityId/tenants/:tenantId")
	{
		tenantScoped.GET("", handler.GetCapability)
		tenantScoped.PUT("", handler.UpdateCapability)
		tenantScoped.DELETE("", handler.DisableCapability)
	}
	if deps.RouterSvc != nil {
		routerHandler := NewRouterHandler(deps.RouterSvc)
		routerGroup := protectedGroup.Group("/admin/router")
		{
			routerGroup.POST("/invoke", routerHandler.Invoke)
			routerGroup.POST("/health", routerHandler.ReportHealth)
		}
		if deps.RouterSandboxSvc != nil {
			sandboxHandler := NewSandboxHandler(deps.RouterSandboxSvc)
			routerGroup.POST("/sandbox/invoke", sandboxHandler.Invoke)
		}
	}
}
