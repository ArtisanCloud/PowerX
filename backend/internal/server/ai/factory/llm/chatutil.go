package llm

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
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
	startedAt := time.Now()
	result, invokeErr := client.Invoke(callCtx, callConfig, prompt)
	logProviderCall(callCtx, callConfig, "invoke", startedAt, invokeErr)
	return result, invokeErr
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
	startedAt := time.Now()
	result, streamErr := client.Stream(callCtx, callConfig, prompt, onDelta)
	logProviderCall(callCtx, callConfig, "stream", startedAt, streamErr)
	return result, streamErr
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
		startedAt := time.Now()
		final, err := cli.Stream(callCtx, callConfig, prompt, onDelta)
		logProviderCall(callCtx, callConfig, "stream", startedAt, err)
		if err == nil {
			return final, nil
		}
		if err != nil && !errors.Is(err, core.ErrStreamNotSupported) {
			return "", err
		}
		// 不支持流式：回退
	}

	// 一次性 + 模拟 token（如需要）
	startedAt := time.Now()
	result, err := cli.Invoke(callCtx, callConfig, prompt)
	logProviderCall(callCtx, callConfig, "invoke", startedAt, err)
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

func logProviderCall(ctx context.Context, mc *config.ModelConfig, mode string, startedAt time.Time, err error) {
	provider, model := "", ""
	timeoutMS := int64(0)
	if mc != nil {
		provider = mc.Provider
		model = mc.Model
		timeoutMS = mc.Timeout.Milliseconds()
	}
	elapsedMS := time.Since(startedAt).Milliseconds()
	if err == nil {
		logger.InfoF(ctx, "[ai.llm.provider] completed provider=%s model=%s mode=%s elapsed_ms=%d request_timeout_ms=%d", provider, model, mode, elapsedMS, timeoutMS)
		return
	}
	var providerErr *core.ProviderCallError
	if errors.As(err, &providerErr) {
		logger.WarnF(ctx, "[ai.llm.provider] failed provider=%s model=%s mode=%s phase=%s endpoint=%s elapsed_ms=%d request_timeout_ms=%d timeout=%t error=%v", providerErr.Provider, providerErr.Model, mode, providerErr.Phase, sanitizeProviderEndpoint(providerErr.Endpoint), providerErr.Elapsed.Milliseconds(), providerErr.RequestTimeout.Milliseconds(), providerErr.IsTimeout(), providerErr.Cause)
		return
	}
	logger.WarnF(ctx, "[ai.llm.provider] failed provider=%s model=%s mode=%s phase=client elapsed_ms=%d request_timeout_ms=%d timeout=%t error=%v", provider, model, mode, elapsedMS, timeoutMS, errors.Is(err, context.DeadlineExceeded), err)
}

func sanitizeProviderEndpoint(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host + parsed.EscapedPath()
}
