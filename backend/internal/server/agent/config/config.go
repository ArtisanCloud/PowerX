package config

import (
	"time"

	einoCfg "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
	mcpconfig "github.com/ArtisanCloud/PowerX/internal/server/mcp/config"
)

const DefaultDriver = "eino"
const AgentSysKey = "agent_system"
const BaseFlowKey = "base_flow"

// RetryPolicyConfig 重试策略配置（避免循环导入）
type RetryPolicyConfig struct {
	MaxAttempts int           `yaml:"max_attempts" json:"max_attempts"`
	Interval    time.Duration `yaml:"interval" json:"interval"`
	Backoff     bool          `yaml:"backoff" json:"backoff"`
	Jitter      bool          `yaml:"jitter" json:"jitter"`
	MaxInterval time.Duration `yaml:"max_interval" json:"max_interval"`
}

// AgentConfig 由调用方提供，用来决定用哪个驱动和基础策略。
type AgentConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int32  `yaml:"port" json:"port"`
	Mode string `yaml:"mode" json:"mode"`

	FlowSpec    FlowSpecConfig `yaml:"flow_spec" json:"flow_spec"`
	TemplateDir string         `yaml:"template_dir" json:"template_dir"`

	// 选用的驱动名称，空则默认 "eino"
	Driver string `yaml:"driver" json:"driver"`

	// MCP 配置（可选）。若为空则使用顶层 mcp 块。
	MCP *mcpconfig.MCPConfig `yaml:"mcp" json:"mcp"`

	// Intent 解析 / prompt 转 plan 所需的上下文（按各驱动解释）
	IntentRecognizer IntentRecognizer `yaml:"intent_recognizer" json:"intent_recognizer"`

	LLMConfig *einoCfg.ModelConfig `yaml:"llm" json:"llm"`

	// 额外上下文 metadata（例如 tenant_id, user_id 等）
	ContextMetadata map[string]interface{} `yaml:"context_metadata" json:"context_metadata"`

	// 失败重试策略，可以为空，驱动内部会合并
	RetryPolicy *RetryPolicyConfig `yaml:"retry_policy" json:"retry_policy"`

	// 调试追踪（兼容保留，建议迁移到 log.agent_debug）：按请求落盘输入/输出。
	DebugTrace DebugTraceConfig `yaml:"debug_trace" json:"debug_trace"`

	// 上下文优化：分层拼装 + token 预算裁剪 + 缓存策略。
	ContextOptimizer ContextOptimizerConfig `yaml:"context_optimizer" json:"context_optimizer"`

	// 可扩展的 driver-specific 选项
	Options map[string]interface{} `yaml:"options" json:"options"`
}

type FlowSpecConfig struct {
	BaseDir     string `yaml:"base_dir" json:"base_dir"`
	BusinessDir string `yaml:"business_dir" json:"business_dir"`
}

type IntentRecognizer struct {
	Rule       RuleConfig       `yaml:"rule" json:"rule"`
	Embedding  EmbeddingConfig  `yaml:"embedding" json:"embedding"`
	Classifier ClassifierConfig `yaml:"classifier" json:"classifier"`
}

type RuleConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type EmbeddingConfig struct {
	Enabled          bool    `yaml:"enabled" json:"enabled"`
	Provider         string  `yaml:"provider" json:"provider"` // openai|ollama
	Endpoint         string  `yaml:"endpoint" json:"endpoint"`
	Model            string  `yaml:"model" json:"model"`
	APIKey           string  `yaml:"api_key" json:"api_key"`
	MaxBatch         int     `yaml:"max_batch" json:"max_batch"`
	Dim              int     `yaml:"dim" json:"dim"` // 可选
	AcceptThreshold  float64 `yaml:"accept_threshold" json:"accept_threshold"`
	LLMGateThreshold float64 `yaml:"llm_gate_threshold" json:"llm_gate_threshold"`
	TopDeltaGate     float64 `yaml:"top_delta_gate" json:"top_delta_gate"`
}

type ClassifierConfig struct {
	Enabled          bool    `yaml:"enabled" json:"enabled"`
	Provider         string  `yaml:"provider" json:"provider"` // openai|ollama
	Endpoint         string  `yaml:"endpoint" json:"endpoint"`
	Model            string  `yaml:"model" json:"model"`
	APIKey           string  `yaml:"api_key" json:"api_key"`
	Temperature      float64 `yaml:"temperature" json:"temperature"`
	MaxTokens        int     `yaml:"max_tokens" json:"max_tokens"`
	LLMMinConfidence float64 `yaml:"llm_min_confidence" json:"llm_min_confidence"`
	TopK             int     `yaml:"top_k" json:"top_k"`
}

type DebugTraceConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	Dir          string `yaml:"dir" json:"dir"`
	MaxBodyBytes int    `yaml:"max_body_bytes" json:"max_body_bytes"`
}

type ContextOptimizerConfig struct {
	Enabled                   bool   `yaml:"enabled" json:"enabled"`
	MaxPromptTokens           int    `yaml:"max_prompt_tokens" json:"max_prompt_tokens"`
	ReservedCompletionTokens  int    `yaml:"reserved_completion_tokens" json:"reserved_completion_tokens"`
	RecentMessages            int    `yaml:"recent_messages" json:"recent_messages"`
	RetrievalTopK             int    `yaml:"retrieval_top_k" json:"retrieval_top_k"`
	CacheMode                 string `yaml:"cache_mode" json:"cache_mode"` // auto|force_off|force_on
	SummaryRefreshIntervalSec int    `yaml:"summary_refresh_interval_sec" json:"summary_refresh_interval_sec"`
}
