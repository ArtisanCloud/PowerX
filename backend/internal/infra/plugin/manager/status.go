package manager

import "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"

// RuntimeStatus：返回 supervisor.ProcInfo（JSON 友好）
func (m *managerImpl) RuntimeStatus(id string) (supervisor.ProcInfo, bool) {
	if m.sup == nil {
		return supervisor.ProcInfo{ID: id, State: supervisor.ProcStopped}, false
	}
	info, ok := m.sup.Status(id)
	return info, ok
}
