package llm

import (
	"bytes"
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
)

// StreamOrFallback：优先真流式；不支持则使用 ChatOnce 并按 rune 模拟 token
func StreamOrFallback(
	ctx context.Context,
	cli LLMClient,
	mc *config.ModelConfig,
	prompt string,
	onDelta func(string),
) (string, error) {
	// 有回调 → 试图流式
	if onDelta != nil {
		final, err := cli.Stream(ctx, mc, prompt, onDelta)
		if err == nil {
			return final, nil
		}
		if err != nil && err != ErrStreamNotSupported {
			return "", err
		}
		// 不支持流式：回退
	}

	// 一次性 + 模拟 token（如需要）
	final, err := cli.Invoke(ctx, mc, prompt)
	if err != nil {
		return "", err
	}
	if onDelta != nil {
		for _, r := range []rune(final) {
			onDelta(string(r))
		}
	}
	return final, nil
}

func TrimDataPrefix(line []byte) []byte {
	b := bytes.TrimSpace(line)
	if bytes.HasPrefix(b, []byte("data:")) {
		b = b[len("data:"):]
		b = bytes.TrimLeft(b, " \t")
	}
	return b
}
