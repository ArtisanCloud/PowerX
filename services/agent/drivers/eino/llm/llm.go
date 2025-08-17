package llm

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/services/agent/config"

	"strings"
)

// 统一配置（融合运行时请求 + agent 默认配置）
type ModelConfig struct {
	Provider     string
	Endpoint     string
	APIKey       string
	Model        string
	SystemPrompt string
	Temperature  float64
	MaxTokens    int

	// 可选：百度
	AccessToken string
	BaiduAK     string
	BaiduSK     string
}

// 运行时透传（来自 ChatRequest.config）
type RuntimeConfig map[string]any

// LLM 客户端接口（先做一次性回复；要流式时再扩展另一个接口）
type Client interface {
	ChatOnce(ctx context.Context, mc ModelConfig, userMessage string) (string, error)
}

// ——— 工厂 ———

func NewClient(provider string) (Client, error) {
	switch strings.ToLower(provider) {
	case "openai":
		return &openaiClient{}, nil
	case "ollama":
		return &ollamaClient{}, nil
	case "baidu":
		return &baiduClient{}, nil
	default:
		return nil, fmt.Errorf("llm: unsupported provider: %s", provider)
	}
}

// ——— 配置融合 ———
// 以 Agent 默认配置为“底”，运行时覆盖它（req.Config > agent cfg）

func MergeConfig(defaults *config.LLMConfig, rt RuntimeConfig) ModelConfig {
	// 底：agent 配置
	out := ModelConfig{
		Provider:     lowerOr(defaults.Provider, "openai"),
		Endpoint:     strOr(defaults.Endpoint, ""),
		APIKey:       strOr(defaults.APIKey, ""),
		Model:        strOr(defaults.Model, "gpt-3.5-turbo"),
		SystemPrompt: strOr(defaults.Template, "You are a helpful assistant."),
		Temperature:  floatOr(defaults.Temperature, 0.7),
		MaxTokens:    intOr(defaults.MaxTokens, 512),
	}

	// 运行时覆盖
	if rt != nil {
		if v := strings.ToLower(utils.StrFrom(rt["provider"])); v != "" {
			out.Provider = v
		}
		if v := utils.StrFrom(rt["endpoint"]); v != "" {
			out.Endpoint = v
		}
		if v := utils.StrFrom(rt["api_key"]); v != "" {
			out.APIKey = v
		}
		// 兼容 model_name / model
		if v := utils.StrFrom(rt["model_name"]); v == "" {
			if v2 := utils.StrFrom(rt["model"]); v2 != "" {
				out.Model = v2
			}
		} else {
			out.Model = v
		}
		if v := utils.StrFrom(rt["system_prompt"]); v != "" {
			out.SystemPrompt = v
		}
		if v := floatAny(rt["temperature"]); v != nil {
			out.Temperature = *v
		}
		if v := intAny(rt["max_tokens"]); v != nil {
			out.MaxTokens = *v
		}

		// Baidu 兼容字段
		if v := utils.StrFrom(rt["access_token"]); v != "" {
			out.AccessToken = v
		}
		if v := utils.StrFrom(rt["baidu_ak"]); v != "" {
			out.BaiduAK = v
		}
		if v := utils.StrFrom(rt["baidu_sk"]); v != "" {
			out.BaiduSK = v
		}
	}

	// 默认 endpoints
	if out.Endpoint == "" {
		switch out.Provider {
		case "openai":
			out.Endpoint = "https://api.openai.com/v1"
		case "ollama":
			out.Endpoint = "http://localhost:11434"
		case "baidu":
			out.Endpoint = "https://aip.baidubce.com"
		}
	}
	return out
}

// ——— 小工具 —— //
func strOr(s string, def string) string {
	if s != "" {
		return s
	}
	return def
}
func lowerOr(s string, def string) string {
	if s != "" {
		return strings.ToLower(s)
	}
	return def
}
func floatOr(f float64, def float64) float64 {
	if f != 0 {
		return f
	}
	return def
}
func intOr(i int, def int) int {
	if i != 0 {
		return i
	}
	return def
}
func floatAny(v any) *float64 {
	switch x := v.(type) {
	case float64:
		return &x
	case int:
		f := float64(x)
		return &f
	}
	return nil
}
func intAny(v any) *int {
	switch x := v.(type) {
	case int:
		return &x
	case float64:
		i := int(x)
		return &i
	}
	return nil
}
