package deploy

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil || deps.DB == nil {
		return
	}
	h := NewHandler(deps)
	if h == nil {
		return
	}

	g := protected.Group("/admin/deploy")
	g.GET("/releases", RequireOpsPermission(deps, iamsvc.OpsResourceDeploy, iamsvc.OpsActionRead), h.ListReleases)
	g.POST("/releases", RequireOpsPermission(deps, iamsvc.OpsResourceDeploy, iamsvc.OpsActionExecute), h.TriggerRelease)
	g.POST("/rollback", RequireOpsPermission(deps, iamsvc.OpsResourceDeploy, iamsvc.OpsActionRollback), h.TriggerRollback)
	g.GET("/health", RequireOpsPermission(deps, iamsvc.OpsResourceDeploy, iamsvc.OpsActionRead), h.GetHealth)
}
