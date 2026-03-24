package dto

import "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"

// ======= 已有：保持不变 =======

// ChatRequest：同步或“仅意图识别”的入口（HTTP POST）
// 若需要流式，请用 StreamChatRequest 或走 /ws
type ChatRequest struct {
	Message string                 `json:"message" validate:"required" example:"你好" description:"聊天消息内容"`
	Config  *ChatConfig            `json:"config,omitempty" description:"聊天配置"`
	Context map[string]interface{} `json:"context,omitempty" description:"上下文信息"`
}

// ChatConfig：保留，用于 LLM 与执行
type ChatConfig struct {
	ModelName    string  `json:"model_name,omitempty" example:"gpt-4o" description:"模型名称"`
	Provider     string  `json:"provider,omitempty" example:"openai" description:"提供商"`
	Endpoint     string  `json:"endpoint,omitempty" example:"https://api.openai.com/v1" description:"API端点"`
	APIKey       string  `json:"api_key,omitempty" description:"API密钥"`
	Temperature  float64 `json:"temperature,omitempty" validate:"omitempty,min=0,max=2" example:"0.7" description:"温度参数"`
	MaxTokens    int     `json:"max_tokens,omitempty" validate:"omitempty,min=1,max=8192" example:"2000" description:"最大令牌数"`
	SystemPrompt string  `json:"system_prompt,omitempty" description:"系统提示词"`
	EnableStream bool    `json:"enable_stream,omitempty" description:"是否启用流式响应"`
	CacheMode    string  `json:"cache_mode,omitempty" description:"提示词缓存策略: auto|force_off|force_on"`
}

// ======= 新增/优化：执行/路由选项 =======

// RouteOptions：仅意图识别或候选数量、阈值控制
type RouteOptions struct {
	RouteOnly  bool    `json:"route_only,omitempty" description:"仅做意图识别，不执行 flow"`
	TopK       int     `json:"top_k,omitempty" description:"候选 flows 数（用于 LLM 分类）" example:"5"`
	LowThresh  float64 `json:"low_thresh,omitempty" description:"低置信度阈值（触发LLM判别）" example:"0.6"`
	HighThresh float64 `json:"high_thresh,omitempty" description:"高置信度阈值（直接命中）" example:"0.78"`
}

// ExecOptions：执行相关
type ExecOptions struct {
	FlowID        string                 `json:"flow_id,omitempty" description:"指定 flow（可选，通常由路由产出）"`
	Params        map[string]interface{} `json:"params,omitempty" description:"流程参数（等价 flowschema.Context）"`
	Meta          map[string]interface{} `json:"meta,omitempty" description:"执行元信息（tenant/user/trace等）"`
	Transport     string                 `json:"transport,omitempty" description:"执行通道: auto|sync|sse|ws" example:"auto"`
	TimeoutSec    int                    `json:"timeout_sec,omitempty" description:"执行超时（秒）" example:"60"`
	ExpectActions bool                   `json:"expect_actions,omitempty" description:"允许后端下发前端动作"`
}

// Slash 指令（/xxx ...）解析后的结构（服务端自动识别）
type SlashCommand struct {
	Raw     string            `json:"raw,omitempty"`
	Name    string            `json:"name,omitempty" example:"reload"`
	Args    map[string]string `json:"args,omitempty"`
	IsValid bool              `json:"is_valid"`
}

// ======= 新：流式入口 =======

// StreamChatRequest：SSE 或 WS 使用的初始请求
type StreamChatRequest struct {
	Message string                 `json:"message" validate:"required"`
	FlowID  string                 `json:"flow_id,omitempty"`
	Config  *ChatConfig            `json:"config,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
	Route   *RouteOptions          `json:"route,omitempty"`
	Exec    *ExecOptions           `json:"exec,omitempty"`
}

// ChatData：你已有的标准回复体，继续保留
type ChatData struct {
	Content   string                 `json:"content" example:"你好！我是AI助手"`
	Role      string                 `json:"role" example:"assistant"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp int64                  `json:"timestamp"`
}

// IntentResult：沿用 services/agent/schemas.IntentResult（不重复定义）

// ChatResponse：同步返回（非流式）
type ChatResponse struct {
	Data        *ChatData             `json:"data,omitempty"`
	Intent      *schemas.IntentResult `json:"intent,omitempty"`
	Actions     []FrontendAction      `json:"actions,omitempty"` // 后端下发给前端执行
	ExecutionID string                `json:"execution_id,omitempty"`
}

// 前端动作（后端期望前端执行的 UI 行为）
type FrontendAction struct {
	Type    string                 `json:"type" example:"highlight|navigate|open_modal|copy_to_clipboard"`
	Payload map[string]interface{} `json:"payload,omitempty"`
	// 可选：幂等键、优先级等
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}
