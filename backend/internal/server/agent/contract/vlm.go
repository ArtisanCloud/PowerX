package contract

import "context"

// VLM (Vision-Language Model): image/text -> text
// 与 LLM 区分，便于路由与能力标注。
type VLMClient interface {
	Invoke(ctx context.Context, in VLMRequest) (*VLMResponse, error)
	Stream(ctx context.Context, in VLMRequest, onDelta func(string)) (*VLMResponse, error)

	Cap() ModelCapabilities
	Health(ctx context.Context) error
}

type VLMRequest struct {
	Messages    []Message
	Temperature float64
	TopP        float64
	MaxTokens   int
	JSONMode    bool
	Runtime     map[string]any
}

type VLMResponse struct {
	Text            string
	Usage           map[string]int
	Provider, Model string
}
