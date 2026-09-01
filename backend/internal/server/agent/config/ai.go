package config

import (
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// AIConfig 对应主配置里的 `ai:` 段
// 仅做“配置结构体”，不碰持久化，不和 repo/service 交叉，避免重复定义。
type AIConfig struct {
	// Provider 清单加载（目录/内置/热更新）
	Catalog catalog.CatalogConfig `yaml:"catalog" mapstructure:"catalog"`

	// 默认调用配置（可选；用于“全局默认值”，被租户/策略/运行时覆盖）
	Defaults AIDefaults `yaml:"defaults" mapstructure:"defaults"`

	// 路由/缓存/容错（可选）
	Routing AIRouting `yaml:"routing" mapstructure:"routing"`
}

// ---------- Global AI Config (read-only snapshot) ----------
//
// 用于在运行时（service/handler）读取 config.yaml 里的 ai.defaults 兜底，
// 避免在 service 层引入顶层 config 包导致循环依赖。
var globalAIConfig atomic.Value // stores *AIConfig

func SetGlobalAIConfig(cfg *AIConfig) {
	if cfg == nil {
		return
	}
	// 存一份指针快照（只读使用）
	globalAIConfig.Store(cfg)
}

func GetGlobalAIConfig() *AIConfig {
	if v := globalAIConfig.Load(); v != nil {
		if cfg, ok := v.(*AIConfig); ok {
			return cfg
		}
	}
	return nil
}

// ---------- Defaults ----------

type AIDefaults struct {
	LLM       LLMDefaults       `yaml:"llm"       mapstructure:"llm"`
	Embedding EmbeddingDefaults `yaml:"embedding" mapstructure:"embedding"`
	Image     ImageDefaults     `yaml:"image"     mapstructure:"image"`
	Video     VideoDefaults     `yaml:"video"     mapstructure:"video"`
}

// BaseConn 与前端“通用”卡片字段一致（provider/endpoint/model/apiKey/...）
type BaseConn struct {
	Provider        string `yaml:"provider"        mapstructure:"provider"`
	Endpoint        string `yaml:"endpoint"        mapstructure:"endpoint"`
	Model           string `yaml:"model"           mapstructure:"model"`
	APIKey          string `yaml:"api_key"         mapstructure:"api_key"`
	Region          string `yaml:"region"          mapstructure:"region"`
	Organization    string `yaml:"organization"    mapstructure:"organization"`
	AzureDeployment string `yaml:"azure_deployment" mapstructure:"azure_deployment"`
}

type LLMDefaults struct {
	BaseConn    `yaml:",inline" mapstructure:",squash"`
	Temperature float64 `yaml:"temperature" mapstructure:"temperature"`
	MaxTokens   int     `yaml:"max_tokens"  mapstructure:"max_tokens"`
	TopP        float64 `yaml:"top_p"       mapstructure:"top_p"`
	Stream      bool    `yaml:"stream"      mapstructure:"stream"`
	// RequestTimeout is the sole per-request deadline for all LLM invocation
	// entry points, including Agent chat, team orchestration and connection tests.
	RequestTimeout time.Duration `yaml:"request_timeout" mapstructure:"request_timeout"`
}

type EmbeddingDefaults struct {
	BaseConn   `yaml:",inline" mapstructure:",squash"`
	Dimensions int    `yaml:"dimensions" mapstructure:"dimensions"`
	Truncate   string `yaml:"truncate"   mapstructure:"truncate"` // none|start|end
	Batch      int    `yaml:"batch"      mapstructure:"batch"`
}

type ImageDefaults struct {
	BaseConn   `yaml:",inline" mapstructure:",squash"`
	Size       string `yaml:"size"       mapstructure:"size"`    // 256x256|512x512|1024x1024
	Quality    string `yaml:"quality"    mapstructure:"quality"` // standard|hd
	Format     string `yaml:"format"     mapstructure:"format"`  // png|jpeg|webp
	PromptHint string `yaml:"prompt_hint" mapstructure:"prompt_hint"`
}

type VideoDefaults struct {
	BaseConn       `yaml:",inline" mapstructure:",squash"`
	Resolution     string `yaml:"resolution"      mapstructure:"resolution"` // 720p|1080p|4k
	FPS            int    `yaml:"fps"             mapstructure:"fps"`
	MaxDurationSec int    `yaml:"max_duration_sec" mapstructure:"max_duration_sec"`
	PromptHint     string `yaml:"prompt_hint"     mapstructure:"prompt_hint"`
}

// ---------- Routing / 缓存 / 容错 ----------

type AIRouting struct {
	Enable               bool          `yaml:"enable"                 mapstructure:"enable"`
	PolicyRefresh        time.Duration `yaml:"policy_refresh"         mapstructure:"policy_refresh"`         // e.g. "30s"
	ProviderCacheTTL     time.Duration `yaml:"provider_cache_ttl"     mapstructure:"provider_cache_ttl"`     // e.g. "5m"
	RouteDecisionTimeout time.Duration `yaml:"route_decision_timeout" mapstructure:"route_decision_timeout"` // e.g. "500ms"
	RetryOnTimeout       bool          `yaml:"retry_on_timeout"       mapstructure:"retry_on_timeout"`
	MaxRetries           int           `yaml:"max_retries"            mapstructure:"max_retries"`
}

// ---------- 默认值填充 ----------

func (c *AIConfig) SetDefaults() {
	// Catalog
	if len(c.Catalog.Dirs) == 0 {
		// 优先使用发行目录（POWERX_LINKS_ROOT/backend/config/...），
		// 未配置时回退到开发态相对路径。
		if linksRoot := strings.TrimSpace(os.Getenv("POWERX_LINKS_ROOT")); linksRoot != "" {
			c.Catalog.Dirs = []string{filepath.Join(linksRoot, "backend", "config", "agents", "providers.d")}
		} else {
			c.Catalog.Dirs = []string{"./config/agents/providers.d"}
		}
	}
	// Defaults.LLM
	if c.Defaults.LLM.Temperature == 0 {
		c.Defaults.LLM.Temperature = 0.7
	}
	if c.Defaults.LLM.MaxTokens == 0 {
		c.Defaults.LLM.MaxTokens = 512
	}
	if c.Defaults.LLM.TopP == 0 {
		c.Defaults.LLM.TopP = 1.0
	}
	if c.Defaults.LLM.RequestTimeout <= 0 {
		c.Defaults.LLM.RequestTimeout = 5 * time.Minute
	}
	// Embedding
	if c.Defaults.Embedding.Batch == 0 {
		c.Defaults.Embedding.Batch = 32
	}
	// Image
	if c.Defaults.Image.Size == "" {
		c.Defaults.Image.Size = "1024x1024"
	}
	if c.Defaults.Image.Quality == "" {
		c.Defaults.Image.Quality = "standard"
	}
	if c.Defaults.Image.Format == "" {
		c.Defaults.Image.Format = "png"
	}
	// Video
	if c.Defaults.Video.Resolution == "" {
		c.Defaults.Video.Resolution = "1080p"
	}
	if c.Defaults.Video.FPS == 0 {
		c.Defaults.Video.FPS = 24
	}
	if c.Defaults.Video.MaxDurationSec == 0 {
		c.Defaults.Video.MaxDurationSec = 10
	}
	// Routing
	if c.Routing.PolicyRefresh == 0 {
		c.Routing.PolicyRefresh = 30 * time.Second
	}
	if c.Routing.ProviderCacheTTL == 0 {
		c.Routing.ProviderCacheTTL = 5 * time.Minute
	}
	if c.Routing.RouteDecisionTimeout == 0 {
		c.Routing.RouteDecisionTimeout = 500 * time.Millisecond
	}
	if c.Routing.MaxRetries == 0 {
		c.Routing.MaxRetries = 1
	}
}
