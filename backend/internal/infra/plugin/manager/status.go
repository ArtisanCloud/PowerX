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

type RuntimeProcessView struct {
	ProcessID string              `json:"process_id"`
	Role      string              `json:"role"`
	Shared    bool                `json:"shared"`
	Info      supervisor.ProcInfo `json:"info"`
}

func (m *managerImpl) RuntimeProcesses(id string) []RuntimeProcessView {
	processes := make([]RuntimeProcessView, 0, 2)
	if m == nil || m.sup == nil {
		return processes
	}
	if info, ok := m.sup.Status(id); ok {
		processes = append(processes, RuntimeProcessView{
			ProcessID: id,
			Role:      "backend",
			Shared:    true,
			Info:      info,
		})
	}
	adminID := id + "_admin"
	if info, ok := m.sup.Status(adminID); ok {
		processes = append(processes, RuntimeProcessView{
			ProcessID: adminID,
			Role:      "admin",
			Shared:    true,
			Info:      info,
		})
	}
	return processes
}
