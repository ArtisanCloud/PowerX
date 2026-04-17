package plugin

import (
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func PluginStatusHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
		return
	}

	ctx := c.Request.Context()
	mgr, err := tryGetPluginManager()
	if err != nil {
		p, getErr := getPluginFromRegistry(ctx, id)
		if getErr != nil {
			dtoRequest.ResponseError(c, 404, "插件不存在", getErr)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{
			"id":      id,
			"version": p.Version,
			"state":   string(p.State),
			"runtime": gin.H{
				"state": "unknown",
			},
			"fallback": true,
		})
		return
	}

	// 注册表视图：版本/状态
	p, err := mgr.Get(ctx, id)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "插件不存在", err)
		return
	}

	// 运行态（supervisor）
	proc, _ := mgrimpl.TryRuntimeStatus(mgr, id)

	dtoRequest.ResponseSuccess(c, gin.H{
		"id":      id,
		"version": p.Version,
		"state":   string(p.State),
		"runtime": gin.H{
			"pid":           proc.PID,
			"port":          proc.Port,
			"state":         proc.State, // starting/running/unhealthy/stopped/exited
			"healthy":       proc.Healthy,
			"restarts":      proc.Restarts,
			"started_at":    proc.StartedAt,
			"stopped_at":    proc.StoppedAt,
			"last_exit_err": proc.LastExitErr,
			"health_ok":     proc.HealthOKCount,
			"health_fails":  proc.HealthFails,
		},
	})
}
