package archive_ops

import "path/filepath"

func (s *Service) ChangeDirectories() []string {
	if s == nil || s.initErr != nil {
		return nil
	}
	return []string{filepath.Join(s.cfg.Directory, "policies"), filepath.Join(s.cfg.Directory, "targets"), filepath.Join(s.cfg.Directory, "jobs"), filepath.Join(s.cfg.Directory, "restores")}
}
