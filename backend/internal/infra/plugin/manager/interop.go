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
