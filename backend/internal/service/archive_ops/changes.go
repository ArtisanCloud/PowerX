package archive_ops

import (
	"os"
	"path/filepath"
)

func (s *Service) ChangeDirectories() []string {
	if s == nil || s.initErr != nil {
		return nil
	}
	out := []string{filepath.Join(s.cfg.Directory, "target-verifications"), filepath.Join(s.cfg.Directory, "policies"), filepath.Join(s.cfg.Directory, "targets"), filepath.Join(s.cfg.Directory, "jobs"), filepath.Join(s.cfg.Directory, "restores"), filepath.Join(s.cfg.Directory, "secrets"), filepath.Join(s.cfg.Directory, "remote-gc"), filepath.Join(s.cfg.Directory, "interruption-requests"), filepath.Join(s.cfg.Directory, "command-executions"), filepath.Join(s.cfg.Directory, "command-confirmations"), filepath.Join(s.cfg.Directory, "command-confirmation-rejections")}
	if s.vault != nil {
		for _, dir := range s.vault.Paths() {
			if st, e := os.Stat(dir); e == nil && st.IsDir() {
				out = append(out, dir)
			}
		}
	}
	if s.targetAuthority != nil {
		out = append(out, s.targetAuthority.ChangeDirectories()...)
	}
	return out
}
