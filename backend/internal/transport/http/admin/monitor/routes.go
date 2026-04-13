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
	h := NewHandler()
	if h == nil {
		return
	}
	g := protected.Group("/admin/monitor/logs")
	g.GET("/config", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.GetLogConfig)
	g.GET("/query", backupHTTP.RequireOpsPermission(deps, iamsvc.OpsResourceBackup, iamsvc.OpsActionRead), h.QueryLogs)
}
