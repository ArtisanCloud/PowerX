package backup

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/gin-gonic/gin"
)

func registerProtectedRoutes(protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil || deps.DB == nil {
		return
	}
	h := NewHandler(deps)
	if h == nil {
		return
	}

	registerGroup := func(path string) {
		g := protected.Group(path)
		g.GET("/policies", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListPolicies)
		g.POST("/policies", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.UpsertPolicy)
		g.POST("/jobs/run", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerBackupJob)
		g.GET("/jobs", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListBackupJobs)
		g.POST("/cleanup", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerCleanup)
		g.POST("/restore-drills/run", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerRestoreDrill)
	}

	// 兼容旧路径与新合同路径。
	registerGroup("/admin/backup")
	registerGroup("/admin/ops/backup")
}
