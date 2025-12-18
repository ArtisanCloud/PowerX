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

	if protectedGroup != nil && deps.CapabilityCatalogSvc != nil {
		if handler := newCatalogHandler(deps.CapabilityCatalogSvc); handler != nil {
			adminCapabilities := protectedGroup.Group("/admin/capabilities")
			adminCapabilities.GET("", handler.ListCapabilities)
			adminCapabilities.GET("/:capabilityId", handler.GetCapability)
			protectedGroup.GET("/admin/capability-sync/jobs", handler.ListSyncJobs)
		}
	}
	if protectedGroup != nil {
		if workflowHandler := newWorkflowHandler(deps); workflowHandler != nil {
			protectedGroup.GET("/admin/workflow-templates", workflowHandler.ListTemplates)
			protectedGroup.POST("/admin/workflow-templates/:templateId/upgrade", workflowHandler.ApproveTemplateUpgrade)
		}
	}

	if publicGroup != nil && deps.DiscoverySvc != nil {
		discoveryHandler := NewDiscoveryHandler(deps.DiscoverySvc)
		publicGroup.GET("/discovery/:tenant_uuid/:capabilityId", discoveryHandler.GetSnapshot)
		publicGroup.POST("/discovery/sync", discoveryHandler.Sync)
	}

	if protectedGroup == nil || deps.CapabilityRegistrySvc == nil {
		return
	}
	handler := NewAdminHandler(AdminHandlerOptions{
		Service: deps.CapabilityRegistrySvc,
	})
	capabilities := protectedGroup.Group("/admin/capability-registry/capabilities")
	{
		capabilities.POST("", handler.CreateCapability)
	}
	tenantScoped := capabilities.Group("/:capabilityId/tenants/:tenant_uuid")
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
