package manager

// 轻量的内部接口，给 Admin Handler 用，不影响 pkg/plugin_mgr.Manager 契约
func (m *managerImpl) RuntimeLogs(id string, tailBytes int) (string, bool) {
	if m.sup == nil {
		return "", false
	}
	return m.sup.Logs(id, tailBytes)
}
