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
