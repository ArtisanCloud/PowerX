package migration

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

	g := protected.Group("/admin/migration")
	g.POST("/runbooks/run", RequireOpsPermission(deps, iamsvc.OpsResourceMigration, iamsvc.OpsActionExecute), h.TriggerMigration)
	g.GET("/runbooks/:migrationId", RequireOpsPermission(deps, iamsvc.OpsResourceMigration, iamsvc.OpsActionRead), h.GetMigration)
	g.POST("/runbooks/:migrationId/acceptance", RequireOpsPermission(deps, iamsvc.OpsResourceMigration, iamsvc.OpsActionExecute), h.AcceptMigration)
	g.POST("/traffic/switch", RequireOpsPermission(deps, iamsvc.OpsResourceMigration, iamsvc.OpsActionExecute), h.TriggerTrafficSwitch)
}
