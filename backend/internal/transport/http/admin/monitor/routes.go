package monitor

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	backupHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/backup"
	"github.com/gin-gonic/gin"
)

func registerProtectedRoutes(protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil {
		return
	}
	h := NewHandler(deps.DB)
	if h == nil {
		return
	}
	g := protected.Group("/admin/monitor/logs")
	g.GET("/config", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.GetLogConfig)
	g.GET("/query", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.QueryLogs)
	g.GET("/retention/runs", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListRetentionRuns)
	g.POST("/retention/run", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerRetentionRun)
	g.POST("/retention/dry-run", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.TriggerRetentionDryRun)
	g.GET("/retention/export", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ExportRetentionDryRun)
	g.GET("/retention/policy", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.GetRetentionPolicy)
	g.PUT("/retention/policy", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.UpdateRetentionPolicy)
	g.GET("/plugins", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.ListPluginLoggingTargets)
	g.GET("/plugins/:id/policy", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.GetPluginLoggingPolicy)
	g.PUT("/plugins/:id/policy", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.PutPluginLoggingPolicy)
	g.POST("/plugins/:id/probe", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionExecute), h.ProbePluginLoggingPolicy)
}
