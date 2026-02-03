package llm

import (
	"context"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
)

// 运行时透传（来自 ChatRequest.config）
type RuntimeConfig map[string]any

// LLMClient：统一包含同步与流式
type LLMClient interface {
	// 一次性完成（completion）
	Invoke(ctx context.Context, mc *config.ModelConfig, prompt string) (string, error)

	// 流式输出；onDelta 每收到一段增量就回调一次；返回最终完整文本
	Stream(ctx context.Context, mc *config.ModelConfig, prompt string, onDelta func(string)) (string, error)
}

// 对“不支持流式”的 provider：嵌入 NoopStream 即可满足接口
type NoopStream struct{}

func (NoopStream) Stream(ctx context.Context, mc *config.ModelConfig, prompt string, onDelta func(string)) (string, error) {
	return "", core.ErrStreamNotSupported
}
