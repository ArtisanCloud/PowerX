package capability_registry

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 将能力注册管理接口挂载到受保护路由。
func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil || deps == nil || deps.CapabilityRegistrySvc == nil {
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
}
