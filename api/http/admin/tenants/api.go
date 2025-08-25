package tenants

import (
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *bootstrap.Deps) {

	hTenant := NewTenantHandler(deps.TenantSvc)
	gTenant := protectedGroup.Group("/admin/tenants")
	{
		gTenant.GET("", hTenant.ListTenants)
		gTenant.GET("/:id", hTenant.GetTenant)
		gTenant.POST("", hTenant.CreateTenant)
		gTenant.PUT("/upsert", hTenant.UpsertTenant)
		gTenant.DELETE("/:id", hTenant.DeleteTenant)

	}
}
