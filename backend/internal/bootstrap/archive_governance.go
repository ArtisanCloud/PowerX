package bootstrap

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/ArtisanCloud/PowerX/config"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
)

// registeredArchiveLogs 只登记当前实例真实配置的日志来源，不附带目标或启用策略。
func registeredArchiveLogs(cfg *config.Config) []archiveops.Source {
	out := []archiveops.Source{}
	seen := map[string]bool{}
	add := func(s archiveops.Source) {
		if !seen[s.ID] {
			// 与日志写入器使用同一工作目录解释相对配置，再固定到真实目录。
			// 归档服务只接受绝对路径，不能让默认 logs/audit 使整套归档初始化失败。
			if absolute, err := filepath.Abs(s.Directory); err == nil {
				s.Directory = absolute
			}
			s.Directory = bindStorageSourcePath(s.Directory)
			out = append(out, s)
			seen[s.ID] = true
		}
	}
	if cfg.Audit.File.Enable {
		dir := cfg.Audit.File.Dir
		if dir == "" {
			dir = "logs/audit"
		}
		prefix := cfg.Audit.File.FilePrefix
		if prefix == "" {
			prefix = "audit_event"
		}
		add(archiveops.Source{ID: "core-audit-files", Name: "PowerX 审计文件日志", Kind: "audit_logs", Directory: dir, RotationFormat: "dated", ActiveFiles: []string{prefix + "-"}})
	}
	if cfg.LogConfig.File.Enable {
		groups := map[string][]string{}
		for _, path := range []string{cfg.LogConfig.File.InfoFilePath, cfg.LogConfig.File.ErrorFilePath} {
			if path != "" {
				abs, _ := filepath.Abs(path)
				groups[filepath.Dir(abs)] = append(groups[filepath.Dir(abs)], filepath.Base(abs))
			}
		}
		for dir, files := range groups {
			sort.Strings(files)
			id := fmt.Sprintf("core-app-logs-%x", sha256.Sum256([]byte(dir)))[:26]
			add(archiveops.Source{ID: id, Name: "应用文件日志", Kind: "application_logs", Directory: dir, RotationFormat: "lumberjack", ActiveFiles: files})
		}
	}
	for _, s := range cfg.StorageGovernance.Sources {
		if s.InstanceID != "" && s.InstanceID != cfg.EnvironmentID && s.Scope != "shared" {
			continue
		}
		if s.Kind != "audit_logs" && s.Kind != "postgresql_logs" && s.Kind != "application_logs" && s.Kind != "system_logs" && s.Kind != "journal" {
			continue
		}
		if s.RotationFormat == "" {
			continue
		}
		add(archiveops.Source{ID: s.ID, Name: s.Name, Kind: s.Kind, Directory: s.Path, RotationFormat: s.RotationFormat, ActiveFiles: s.ActiveFiles, FileOwnerUID: s.FileOwnerUID, CurrentLogfiles: s.CurrentLogfiles})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
