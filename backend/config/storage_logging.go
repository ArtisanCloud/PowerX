package config

import logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"

// EffectiveLogConfig 在部署明确移交后保留轮转／压缩，关闭原生按天／份数删除。
// 不改写原始配置；暂停新策略时也不能偷偷恢复旧的破坏性保留器。
func (c *Config) EffectiveLogConfig() logcfg.LogConfig {
	out := c.LogConfig
	if c.StorageGovernance.LifecycleEnabled && c.StorageGovernance.ManageNativeRetention {
		out.File.MaxAge = 0
		out.File.MaxBackups = 0
	}
	return out
}
