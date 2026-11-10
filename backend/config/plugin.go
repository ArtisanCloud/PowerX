package config

type PluginConfig struct {
	Enabled        bool   `yaml:"enabled"`          // 开关
	RegistryFile   string `yaml:"registry_file"`    // 插件注册文件，默认 "./plugins/registry.json"
	BasePrefix     string `yaml:"base_prefix"`      // 路由前缀，默认 "/_p"
	InstalledDir   string `yaml:"installed_dir"`    // 安装目录，默认 "./plugins/installed"
	MarketCacheDir string `yaml:"market_cache_dir"` // 市场缓存，默认 "./plugins/market_cache"`
	// 是否允许执行 destructive 数据库清理（DROP SCHEMA/ROLE/USER/DATABASE）。
	// 强烈建议生产保持 false，仅在受控运维窗口下临时开启。
	AllowDestructiveDBCleanup bool `yaml:"allow_destructive_db_cleanup"`
	// 启动时自动恢复已启用插件的并发度（默认1，建议本地2~4）
	AutoRestoreParallelism int `yaml:"auto_restore_parallelism"`
	// EventFabricNamespaceOwners 为旧命名或共享命名指定唯一插件归属；只能由 Core 配置维护。
	EventFabricNamespaceOwners map[string]string `yaml:"event_fabric_namespace_owners"`

	// 预留（里程碑后续用）
	ReadTimeoutSec  int `yaml:"read_timeout_sec"`
	WriteTimeoutSec int `yaml:"write_timeout_sec"`

	DevMode bool `yaml:"dev_mode"` // 开发模式，默认 false
}

func DefaultPluginConfig() PluginConfig {
	return PluginConfig{
		Enabled:                   true,
		BasePrefix:                "/_p",
		InstalledDir:              "./plugins/installed",
		MarketCacheDir:            "./plugins/market_cache",
		AllowDestructiveDBCleanup: false,
		AutoRestoreParallelism:    1,
		ReadTimeoutSec:            15,
		WriteTimeoutSec:           15,
	}
}

// PluginAggregateConfig 聚合所有插件相关模块配置，作为单一入口暴露。
type PluginAggregateConfig struct {
	PluginConfig `yaml:",inline"`
	Marketplace  MarketplaceConfig     `yaml:"marketplace"`
	Release      PluginReleaseConfig   `yaml:"release"`
	DevHotload   DevHotloadConfig      `yaml:"dev_hotload"`
	Bootstrap    PluginBootstrapConfig `yaml:"bootstrap"`
	Debug        PluginDebugConfig     `yaml:"debug"`
}

// DefaultMarketplaceAPIRoot 是新部署的官方市场入口；配置文件和环境变量可覆盖。
const DefaultMarketplaceAPIRoot = "https://marketplace.artisan-cloud.com/api/v1"

// MarketplaceConfig 下载凭证只能保存在服务端环境，不得通过公开配置返回。
type MarketplaceConfig struct {
	APIRoot       string `yaml:"api_root"`
	PublicSiteURL string `yaml:"public_site_url"` // 可选网站入口，与对象存储地址无关。
	DownloadToken string `yaml:"download_token" json:"-"`
	// EnrollmentToken 仅兼容旧版手工接入；正常启动自动登记，无需配置。不导出到 YAML/JSON。
	EnrollmentToken  string `yaml:"-" json:"-"`
	HeartbeatSeconds int    `yaml:"heartbeat_seconds"`
}

// DefaultMarketplaceConfig 统一源码、Setup、容器和分发二进制的市场默认值。
func DefaultMarketplaceConfig() MarketplaceConfig {
	return MarketplaceConfig{APIRoot: DefaultMarketplaceAPIRoot, HeartbeatSeconds: 300}
}

// DefaultPluginAggregateConfig 返回聚合后的默认配置。
func DefaultPluginAggregateConfig() PluginAggregateConfig {
	return PluginAggregateConfig{
		PluginConfig: DefaultPluginConfig(),
		Marketplace:  DefaultMarketplaceConfig(),
		Release:      DefaultPluginReleaseConfig(),
		DevHotload:   DefaultDevHotloadConfig(),
		Bootstrap:    DefaultPluginBootstrapConfig(),
		Debug:        DefaultPluginDebugConfig(),
	}
}
