package archive_ops

// remoteExecutionAllowed 是部署发布范围门槛，不能由 UI 策略或旧执行快照覆盖。
// 它不改变原策略、不删除历史，也不影响本地媒体导出或命令只读诊断。
func (s *Service) remoteExecutionAllowed() error {
	if s.cfg.RemoteExecutionBlocked {
		return failure(423, "ARCHIVE_RELEASE_NOT_ACCEPTED", "本次版本未开放远端归档；请先完成独立目标和恢复验收")
	}
	return nil
}

type RemoteExecutionAvailability struct {
	Enabled bool   `json:"enabled"`
	Problem string `json:"problem,omitempty"`
}
