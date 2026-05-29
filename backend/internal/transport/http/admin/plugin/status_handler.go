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
			"runtime_scope": gin.H{
				"scope":               "global_plugin_process",
				"tenant_isolated":     false,
				"tenant_instance_key": "",
				"process_id_prefix":   id,
			},
			"tenant_instances": gin.H{
				"managed_by": "TenantPluginInstance",
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
	processes, _ := mgrimpl.TryRuntimeProcesses(mgr, id)

	dtoRequest.ResponseSuccess(c, gin.H{
		"id":      id,
		"version": p.Version,
		"state":   string(p.State),
		"runtime_scope": gin.H{
			"scope":               "global_plugin_process",
			"tenant_isolated":     false,
			"shared_by_tenants":   true,
			"tenant_instance_key": "",
			"process_id_prefix":   id,
		},
		"processes": processes,
		"tenant_instances": gin.H{
			"managed_by": "TenantPluginInstance",
			"note":       "租户启用/停用只改变租户实例访问权，不复制或停止全局插件进程",
		},
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
