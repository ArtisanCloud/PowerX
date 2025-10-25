package llm

import "fmt"

// NewClient：按 provider 返回具体实现（都满足 LLMClient）
func NewClient(provider string) (LLMClient, error) {
	switch normalize(provider) {
	case "openai":
		return NewOpenAIClient(), nil
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
	// 简单归一化，省略实现
	return s
}
