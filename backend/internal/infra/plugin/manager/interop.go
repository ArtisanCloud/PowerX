package manager

import (
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

type runtimeStatusIfc interface {
	RuntimeStatus(id string) (supervisor.ProcInfo, bool)
}

func TryRuntimeStatus(mgr plugin_mgr.Manager, id string) (supervisor.ProcInfo, bool) {
	if rs, ok := mgr.(runtimeStatusIfc); ok {
		return rs.RuntimeStatus(id)
	}
	return supervisor.ProcInfo{ID: id, State: supervisor.ProcStopped}, false
}

// InstallationRuntimeReady 同时核对后端及进程型管理前端，不用 registry.enabled 代替运行就绪。
func InstallationRuntimeReady(mgr plugin_mgr.Manager, p plugin_mgr.Plugin) bool {
	api, ok := TryRuntimeStatus(mgr, p.ID)
	if p.State != plugin_mgr.StateEnabled || !ok || !api.Healthy {
		return false
	}
	if p.Frontend.Admin.Kind == plugin_mgr.FrontendKindProcess {
		admin, ok := TryRuntimeStatus(mgr, p.ID+"_admin")
		alive := ok && admin.State != supervisor.ProcStopped && admin.State != supervisor.ProcExited && admin.State != supervisor.ProcCrashed
		if alive && (p.Frontend.Admin.Process == nil || p.Frontend.Admin.Process.Health.HTTPPath == "" || admin.Healthy) {
			return true
		}
		return adminStaticFallbackReady(p)
	}
	return true
}
