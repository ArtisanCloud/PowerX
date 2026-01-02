package agent

import (
	"context"
	"errors"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

// ChatConfigResolver 在聊天执行前把“AI settings 默认 + Agent 自身覆盖 + 本次请求临时配置”
// 汇总成一个可直接用于执行的 dto.ChatConfig（包含 provider/model/endpoint/api_key/参数）。
//
// 生效顺序（低 -> 高）：
// 1) AI settings 默认（GetActiveProfile: tenant/env）
// 2) Agent 自身 AI 覆盖（AgentSetting: tenant/env/agent）
// 3) 本次请求临时配置（dto.ChatConfig）
//
// 节点级 node.params 覆盖在执行器内部处理（优先级最高），不在这里合并。
type ChatConfigResolver struct {
	setting *AgentSettingService
	agent   *AgentService
}

func NewChatConfigResolver(db *gorm.DB) *ChatConfigResolver {
	return &ChatConfigResolver{
		setting: NewAgentSettingService(db),
		agent:   NewAgentService(db),
	}
}

// ResolveForAgentChat 构造本次聊天的有效 LLM 配置。
// 如果系统未配置 AI settings（无可用的 provider/model/credential），会返回可直接展示给用户的错误信息。
func (r *ChatConfigResolver) ResolveForAgentChat(
	ctx context.Context,
	env string,
	tenantUUID *string,
	agentID uint64,
	req *dto.ChatConfig,
) (*dto.ChatConfig, error) {
	out := &dto.ChatConfig{}

	// 1) AI settings 默认（tenant/env）
	base, err := r.buildFromActiveProfile(ctx, env, tenantUUID)
	if err != nil {
		return nil, err
	}
	mergeChatConfig(out, base)

	// 2) Agent 自身覆盖（tenant/env/agent）
	agentCfg, err := r.buildFromAgentSetting(ctx, env, tenantUUID, agentID)
	if err != nil {
		return nil, err
	}
	mergeChatConfig(out, agentCfg)

	// 3) 本次请求临时覆盖
	mergeChatConfig(out, req)

	// 最终校验 & 补全 endpoint/apiKey（仅当缺失时从 credential store 回填）
	if strings.TrimSpace(out.Provider) == "" || strings.TrimSpace(out.ModelName) == "" {
		return nil, errors.New("未配置默认 LLM：请先在「AI Settings」选择并保存一个可用的 Provider/Model（并配置凭据）")
	}
	if err := r.fillConnFromStore(ctx, env, tenantUUID, out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *ChatConfigResolver) buildFromActiveProfile(
	ctx context.Context,
	env string,
	tenantUUID *string,
) (*dto.ChatConfig, error) {
	prof, err := r.setting.GetActiveProfile(ctx, env, tenantUUID, "llm")
	if err != nil || prof == nil {
		return &dto.ChatConfig{}, nil
	}

	out := &dto.ChatConfig{
		Provider:  strings.TrimSpace(prof.Provider),
		ModelName: strings.TrimSpace(prof.Model),
	}
	applyLLMDefaultsFromProfile(out, prof)

	// profile 本身不含凭据，执行时需要补 base_url/api_key
	if strings.TrimSpace(out.Provider) != "" {
		if err := r.fillConnFromStore(ctx, env, tenantUUID, out); err != nil {
			// 对“仅有 config.yaml 默认但没配凭据”的场景：给清晰引导
			return nil, errors.New("AI Settings 未配置或凭据不可用：请先在「AI Settings」保存该 Provider 的连接信息（base_url/api_key），再开始对话")
		}
	}
	return out, nil
}

func (r *ChatConfigResolver) buildFromAgentSetting(
	ctx context.Context,
	env string,
	tenantUUID *string,
	agentID uint64,
) (*dto.ChatConfig, error) {
	if agentID == 0 {
		return &dto.ChatConfig{}, nil
	}
	setting, err := r.agent.GetAgentAISetting(ctx, env, tenantUUID, agentID)
	if err != nil || setting == nil {
		return &dto.ChatConfig{}, nil
	}
	return chatConfigFromAgentSetting(setting), nil
}

func (r *ChatConfigResolver) fillConnFromStore(
	ctx context.Context,
	env string,
	tenantUUID *string,
	c *dto.ChatConfig,
) error {
	if c == nil {
		return nil
	}
	p := strings.TrimSpace(c.Provider)
	if p == "" {
		return nil
	}
	baseURL, apiKey, err := r.setting.ResolveConnFromStore(ctx, env, tenantUUID, p, c.Endpoint, c.APIKey)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		c.Endpoint = baseURL
	}
	if strings.TrimSpace(c.APIKey) == "" {
		c.APIKey = apiKey
	}
	return nil
}

func mergeChatConfig(dst *dto.ChatConfig, src *dto.ChatConfig) {
	if dst == nil || src == nil {
		return
	}
	if strings.TrimSpace(src.Provider) != "" {
		dst.Provider = strings.TrimSpace(src.Provider)
	}
	if strings.TrimSpace(src.ModelName) != "" {
		dst.ModelName = strings.TrimSpace(src.ModelName)
	}
	if strings.TrimSpace(src.Endpoint) != "" {
		dst.Endpoint = strings.TrimSpace(src.Endpoint)
	}
	if strings.TrimSpace(src.APIKey) != "" {
		dst.APIKey = strings.TrimSpace(src.APIKey)
	}
	if strings.TrimSpace(src.SystemPrompt) != "" {
		dst.SystemPrompt = strings.TrimSpace(src.SystemPrompt)
	}
	if src.Temperature > 0 {
		dst.Temperature = src.Temperature
	}
	if src.MaxTokens > 0 {
		dst.MaxTokens = src.MaxTokens
	}
	if src.EnableStream {
		dst.EnableStream = true
	}
}

func applyLLMDefaultsFromProfile(out *dto.ChatConfig, prof *dbmodel.AIModelProfile) {
	if out == nil || prof == nil || prof.Defaults == nil {
		return
	}
	if v, ok := prof.Defaults["temperature"]; ok {
		if f, ok2 := asFloat64(v); ok2 && f > 0 {
			out.Temperature = f
		}
	}
	if v, ok := prof.Defaults["maxTokens"]; ok {
		if i, ok2 := asInt(v); ok2 && i > 0 {
			out.MaxTokens = i
		}
	}
	if v, ok := prof.Defaults["max_tokens"]; ok {
		if i, ok2 := asInt(v); ok2 && i > 0 {
			out.MaxTokens = i
		}
	}
	if v, ok := prof.Defaults["topP"]; ok {
		// dto.ChatConfig 没有 top_p 字段；这里暂不透传，避免“看似配置了但不生效”的误导
		_ = v
	}
	if v, ok := prof.Defaults["stream"]; ok {
		if b, ok2 := v.(bool); ok2 && b {
			out.EnableStream = true
		}
	}
}

func chatConfigFromAgentSetting(setting *dbmodel.AgentSetting) *dto.ChatConfig {
	if setting == nil {
		return &dto.ChatConfig{}
	}
	hasFlags := len(setting.OverrideFlags) > 0
	allow := func(key string) bool {
		if !hasFlags {
			return true
		}
		if v, ok := setting.OverrideFlags[key]; ok {
			if b, ok2 := v.(bool); ok2 {
				return b
			}
		}
		return false
	}

	out := &dto.ChatConfig{}
	if allow("provider") && strings.TrimSpace(setting.Provider) != "" {
		out.Provider = strings.TrimSpace(setting.Provider)
	}
	if allow("model") && strings.TrimSpace(setting.Model) != "" {
		out.ModelName = strings.TrimSpace(setting.Model)
	}

	// params（目前前端仅写 provider/model；这里先做通用支持）
	if setting.Params != nil {
		if allow("temperature") {
			if v, ok := setting.Params["temperature"]; ok {
				if f, ok2 := asFloat64(v); ok2 && f > 0 {
					out.Temperature = f
				}
			}
		}
		if allow("max_tokens") || allow("maxTokens") {
			if v, ok := setting.Params["max_tokens"]; ok {
				if i, ok2 := asInt(v); ok2 && i > 0 {
					out.MaxTokens = i
				}
			}
			if v, ok := setting.Params["maxTokens"]; ok {
				if i, ok2 := asInt(v); ok2 && i > 0 {
					out.MaxTokens = i
				}
			}
		}
		if allow("system_prompt") || allow("systemPrompt") {
			if v, ok := setting.Params["system_prompt"]; ok {
				if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
					out.SystemPrompt = strings.TrimSpace(s)
				}
			}
			if v, ok := setting.Params["systemPrompt"]; ok {
				if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
					out.SystemPrompt = strings.TrimSpace(s)
				}
			}
		}
	}
	return out
}

func asFloat64(v any) (float64, bool) {
	switch vv := v.(type) {
	case float64:
		return vv, true
	case float32:
		return float64(vv), true
	case int:
		return float64(vv), true
	case int64:
		return float64(vv), true
	}
	return 0, false
}

func asInt(v any) (int, bool) {
	switch vv := v.(type) {
	case int:
		return vv, true
	case int64:
		return int(vv), true
	case float64:
		return int(vv), true
	case float32:
		return int(vv), true
	}
	return 0, false
}
