package llm

import (
	"fmt"
	"strings"
)

// NewClient：按 provider 返回具体实现（都满足 LLMClient）
func NewClient(provider string) (LLMClient, error) {
	switch normalize(provider) {
	case "openai":
		return NewOpenAIClient(provider), nil
	case "hunyuan":
		return NewHunyuanClient(), nil
	case "ollama":
		return NewOllamaClient(), nil
	case "baidu", "qianfan":
		return NewBaiduClient(), nil
	// ... 其他厂商
	default:
		return nil, fmt.Errorf("unknown llm provider: %s", provider)
	}
}

func normalize(s string) string {
	p := strings.ToLower(strings.TrimSpace(s))
	if p == "" {
		return ""
	}
	// OpenAI-compatible providers → reuse openai client
	switch p {
	case "openrouter", "vllm", "deepseek", "moonshot":
		return "openai"
	}
	return p
}
