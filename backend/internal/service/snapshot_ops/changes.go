package snapshot_ops

import "path/filepath"

// ChangeDirectories 仅供宿主通知装配，不通过 HTTP 返回私有来源路径。
func (s *Service) ChangeDirectories() []string {
	if s == nil || s.initErr != nil {
		return nil
	}
	return []string{s.cfg.Directory, filepath.Join(s.cfg.Directory, "snapshots"), filepath.Join(s.cfg.Directory, "restores")}
}

func (c *CoordinatorStore) ChangeDirectories() []string {
	if c == nil {
		return nil
	}
	return []string{c.Directory, filepath.Join(c.Directory, "jobs")}
}
