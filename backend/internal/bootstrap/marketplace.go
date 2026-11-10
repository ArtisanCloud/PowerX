package bootstrap

import (
	"github.com/ArtisanCloud/PowerX/config"
	"path/filepath"
)

// 使用环境管理已有的私有目录，不把市场身份复制到插件包或业务数据库。
func marketplaceStateDirectory(cfg *config.Config) string {
	if cfg.EnvironmentIdentityFile() == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cfg.EnvironmentIdentityFile()), "marketplace")
}
