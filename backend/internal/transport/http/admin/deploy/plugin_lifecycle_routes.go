package deploy

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/gin-gonic/gin"
)

func registerPluginLifecycleRoutes(protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil || deps.DB == nil {
		return
	}
	h := NewPluginLifecycleHandler(deps)
	if h == nil {
		return
	}

	g := protected.Group("/admin/plugins")
	g.GET("/:pluginId/audit", RequireOpsPermission(deps, iamsvc.OpsResourcePlugin, iamsvc.OpsActionRead), h.ListPluginLifecycleAudits)
	g.POST("/:pluginId/actions", RequireOpsPermission(deps, iamsvc.OpsResourcePlugin, iamsvc.OpsActionExecute), h.TriggerPluginLifecycleAction)
}
