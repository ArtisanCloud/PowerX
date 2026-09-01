package llm

import (
	"context"
	"fmt"
	"time"

	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
)

// RequestTimeout returns the single configured LLM request deadline. It is
// deliberately independent from an Engine's whole-run deadline.
func RequestTimeout() time.Duration {
	if ai := agentconfig.GetGlobalAIConfig(); ai != nil && ai.Defaults.LLM.RequestTimeout > 0 {
		return ai.Defaults.LLM.RequestTimeout
	}
	return 5 * time.Minute
}

func withRequestPolicy(ctx context.Context, mc *config.ModelConfig) (context.Context, *config.ModelConfig, context.CancelFunc, error) {
	if mc == nil {
		return nil, nil, nil, fmt.Errorf("llm invocation requires model config")
	}
	if mc.Provider == "" || mc.Model == "" {
		return nil, nil, nil, fmt.Errorf("llm invocation requires provider and model")
	}
	timeout := RequestTimeout()
	if timeout <= 0 {
		return nil, nil, nil, fmt.Errorf("llm request timeout must be configured")
	}
	copy := *mc
	copy.Timeout = timeout
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	return callCtx, &copy, cancel, nil
}

// Invoke is the only synchronous LLM invocation entry point. Provider drivers
// only adapt protocols; request timeout ownership stays here.
func Invoke(ctx context.Context, mc *config.ModelConfig, prompt string) (*config.InvokeResult, error) {
	callCtx, callConfig, cancel, err := withRequestPolicy(ctx, mc)
	if err != nil {
		return nil, err
	}
	defer cancel()
	client, err := NewClient(callConfig.Provider)
	if err != nil {
		return nil, err
	}
	return client.Invoke(callCtx, callConfig, prompt)
}

// Stream is the only native-stream LLM invocation entry point.
func Stream(ctx context.Context, mc *config.ModelConfig, prompt string, onDelta func(string)) (string, error) {
	callCtx, callConfig, cancel, err := withRequestPolicy(ctx, mc)
	if err != nil {
		return "", err
	}
	defer cancel()
	client, err := NewClient(callConfig.Provider)
	if err != nil {
		return "", err
	}
	return client.Stream(callCtx, callConfig, prompt, onDelta)
}

// StreamOrFallback prefers native streaming and otherwise replays a completed
// reply. Both branches share the same configured request policy.
func StreamOrFallback(
	ctx context.Context,
	mc *config.ModelConfig,
	prompt string,
	onDelta func(string),
) (string, error) {
	callCtx, callConfig, cancel, err := withRequestPolicy(ctx, mc)
	if err != nil {
		return "", err
	}
	defer cancel()
	cli, err := NewClient(callConfig.Provider)
	if err != nil {
		return "", err
	}
	// 有回调 → 试图流式
	if onDelta != nil {
		final, err := cli.Stream(callCtx, callConfig, prompt, onDelta)
		if err == nil {
			return final, nil
		}
		if err != nil && err != core.ErrStreamNotSupported {
			return "", err
		}
		// 不支持流式：回退
	}

	// 一次性 + 模拟 token（如需要）
	result, err := cli.Invoke(callCtx, callConfig, prompt)
	if err != nil {
		return "", err
	}
	final := ""
	if result != nil {
		final = result.Text
	}
	if onDelta != nil {
		for _, r := range []rune(final) {
			onDelta(string(r))
		}
	}
	return final, nil
}
