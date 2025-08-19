package manager

// RuntimeStatus 对 Admin 暴露一个不稳定接口（仅用于本进程内的查询）
func (m *managerImpl) RuntimeStatus(id string) (any, bool) {
	if m.sup == nil {
		return nil, false
	}
	info, ok := m.sup.Status(id)
	if !ok {
		// 返回一个统一结构也行，这里直接 false 让上层兜底
		return nil, false
	}
	// 直接返回 supervisor.ProcInfo（JSON 友好）
	return info, true
}
