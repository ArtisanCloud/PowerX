package llm

import (
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/baidu"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/hunyuan"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/ollama"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/openai"
)

// NewClient：按 provider 返回具体实现（都满足 LLMClient）
func NewClient(provider string) (LLMClient, error) {
	switch normalize(provider) {
	case "openai":
		return openai.NewLLMClient(provider), nil
	case "hunyuan":
		return hunyuan.NewLLMClient(), nil
	case "ollama":
		return ollama.NewLLMClient(), nil
	case "baidu", "qianfan":
		return baidu.NewLLMClient(), nil
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
	case "openrouter", "vllm", "deepseek", "moonshot", "huggingface", "hf":
		return "openai"
	}
	return p
}
