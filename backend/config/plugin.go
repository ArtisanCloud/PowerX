package config

type PluginConfig struct {
	Enabled        bool   `yaml:"enabled"`          // 开关
	RegistryFile   string `yaml:"registry_file"`    // 插件注册文件，默认 "./plugins/registry.json"
	BasePrefix     string `yaml:"base_prefix"`      // 路由前缀，默认 "/_p"
	InstalledDir   string `yaml:"installed_dir"`    // 安装目录，默认 "./plugins/installed"
	MarketCacheDir string `yaml:"market_cache_dir"` // 市场缓存，默认 "./plugins/market_cache"`

	// 预留（里程碑后续用）
	ReadTimeoutSec  int `yaml:"read_timeout_sec"`
	WriteTimeoutSec int `yaml:"write_timeout_sec"`

	DevMode bool `yaml:"dev_mode"` // 开发模式，默认 false
}

func DefaultPluginConfig() PluginConfig {
	return PluginConfig{
		Enabled:         false,
		BasePrefix:      "/_p",
		InstalledDir:    "./plugins/installed",
		MarketCacheDir:  "./plugins/market_cache",
		ReadTimeoutSec:  15,
		WriteTimeoutSec: 15,
	}
}

// PluginAggregateConfig 聚合所有插件相关模块配置，作为单一入口暴露。
type PluginAggregateConfig struct {
	PluginConfig `yaml:",inline"`
	Release      PluginReleaseConfig   `yaml:"release"`
	DevHotload   DevHotloadConfig      `yaml:"dev_hotload"`
	Bootstrap    PluginBootstrapConfig `yaml:"bootstrap"`
	Debug        PluginDebugConfig     `yaml:"debug"`
}

// DefaultPluginAggregateConfig 返回聚合后的默认配置。
func DefaultPluginAggregateConfig() PluginAggregateConfig {
	return PluginAggregateConfig{
		PluginConfig: DefaultPluginConfig(),
		Release:      DefaultPluginReleaseConfig(),
		DevHotload:   DefaultDevHotloadConfig(),
		Bootstrap:    DefaultPluginBootstrapConfig(),
		Debug:        DefaultPluginDebugConfig(),
	}
}
