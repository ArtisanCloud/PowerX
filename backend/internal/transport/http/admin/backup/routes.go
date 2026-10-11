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

	g := protected.Group("/admin/ops/backup")
	g.GET("/archives", RequireRestoreRoot(), h.ArchiveOverview)
	g.POST("/archives/policies", RequireRestoreRoot(), h.SaveArchivePolicy)
	g.POST("/archives/targets", RequireRestoreRoot(), h.SaveArchiveTarget)
	g.POST("/archives/targets/:target_id/test", RequireRestoreRoot(), h.TestArchiveTarget)
	g.POST("/archives/policies/:policy_id/run", RequireRestoreRoot(), h.RunArchive)
	g.GET("/archives/jobs/:job_id", RequireRestoreRoot(), h.GetArchive)
	g.POST("/archives/jobs/:job_id/retry", RequireRestoreRoot(), h.RetryArchive)
	g.PATCH("/archives/jobs/:job_id/protection", RequireRestoreRoot(), h.ProtectArchive)
	g.GET("/archives/jobs/:job_id/download", RequireRestoreRoot(), h.DownloadArchive)
	g.POST("/archives/jobs/:job_id/restore", RequireRestoreRoot(), h.RestoreArchive)
	g.POST("/archives/cleanup", RequireRestoreRoot(), h.CleanupArchives)
	g.GET("/archives/restores/:restore_id", RequireRestoreRoot(), h.GetArchiveRestore)
	g.DELETE("/archives/restores/:restore_id", RequireRestoreRoot(), h.DeleteArchiveRestore)
	g.POST("/restore-copies/imports", RequireRestoreRoot(), h.ImportRestoreCopy)
	g.POST("/restore-copies/plan", RequireRestoreRoot(), h.PlanRestoreCopy)
	g.POST("/restore-copies", RequireRestoreRoot(), h.CreateRestoreCopy)
	g.GET("/restore-copies", RequireRestoreRoot(), h.ListRestoreCopies)
	g.GET("/restore-copies/:copy_id", RequireRestoreRoot(), h.GetRestoreCopy)
	g.POST("/restore-copies/:copy_id/connection", RequireRestoreRoot(), h.RestoreCopyConnection)
	g.DELETE("/restore-copies/:copy_id", RequireRestoreRoot(), h.DeleteRestoreCopy)
	g.GET("/policies", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListPolicies)
	g.POST("/policies", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.CreatePolicy)
	g.PATCH("/policies/:policy_id", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.UpdatePolicy)
	g.POST("/policies/:policy_id/enable", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.EnablePolicy)
	g.POST("/policies/:policy_id/disable", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.DisablePolicy)
	g.POST("/policies/:policy_id/current", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.SetCurrentPolicy)
	g.POST("/jobs/run", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerBackupJob)
	g.GET("/jobs", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListBackupJobs)
	g.PATCH("/jobs/:job_id/protection", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.SetJobProtected)
	g.GET("/jobs/:job_id", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.GetBackupJob)
	g.POST("/cleanup", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerCleanup)
	g.POST("/targets/test", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TestTargetConnection)
	g.POST("/restore-drills/run", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerRestoreDrill)
	g.POST("/restore-drills", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerRestoreDrill)
	g.GET("/restore-drills", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListRestoreDrills)
	g.GET("/restore-drills/:drill_id", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.GetRestoreDrill)
	g.GET("/alerts", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListAlerts)
	g.POST("/alerts/:alert_id/ack", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.AckAlert)
	legacy := protected.Group("/admin/backup")
	legacy.GET("/policies", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListPolicies)
	legacy.POST("/policies", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.CreatePolicy)
	legacy.PATCH("/policies/:policy_id", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.UpdatePolicy)
	legacy.POST("/policies/:policy_id/enable", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.EnablePolicy)
	legacy.POST("/policies/:policy_id/disable", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.DisablePolicy)
	legacy.POST("/policies/:policy_id/current", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.SetCurrentPolicy)
	legacy.POST("/jobs/run", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerBackupJob)
	legacy.GET("/jobs", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListBackupJobs)
	legacy.PATCH("/jobs/:job_id/protection", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.SetJobProtected)
	legacy.GET("/jobs/:job_id", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.GetBackupJob)
	legacy.POST("/cleanup", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerCleanup)
	legacy.POST("/targets/test", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TestTargetConnection)
	legacy.POST("/restore-drills/run", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerRestoreDrill)
	legacy.POST("/restore-drills", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerRestoreDrill)
	legacy.GET("/restore-drills", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListRestoreDrills)
	legacy.GET("/restore-drills/:drill_id", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.GetRestoreDrill)
	legacy.GET("/alerts", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListAlerts)
	legacy.POST("/alerts/:alert_id/ack", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.AckAlert)

	monitor := protected.Group("/admin/monitor/backup")
	monitor.GET("/overview", RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.GetBackupOverview)
}
