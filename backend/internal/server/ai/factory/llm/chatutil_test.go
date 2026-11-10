package llm

import (
	"context"
	"testing"
	"time"

	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	aiconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
)

func TestWithRequestPolicyUsesConfiguredUnifiedTimeout(t *testing.T) {
	agentconfig.SetGlobalAIConfig(&agentconfig.AIConfig{
		Defaults: agentconfig.AIDefaults{LLM: agentconfig.LLMDefaults{RequestTimeout: 5 * time.Minute}},
	})
	ctx, modelConfig, cancel, err := withRequestPolicy(context.Background(), &aiconfig.ModelConfig{
		Provider: "ollama",
		Model:    "qwen3:8b",
		Timeout:  20 * time.Second,
	})
	if err != nil {
		t.Fatalf("withRequestPolicy returned error: %v", err)
	}
	defer cancel()
	if modelConfig.Timeout != 5*time.Minute {
		t.Fatalf("timeout = %s, want 5m", modelConfig.Timeout)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("call context must have a deadline")
	}
	remaining := time.Until(deadline)
	if remaining < 4*time.Minute+59*time.Second || remaining > 5*time.Minute {
		t.Fatalf("deadline remaining = %s, want approximately 5m", remaining)
	}
}

func TestWithRequestPolicyRequiresModelIdentity(t *testing.T) {
	_, _, _, err := withRequestPolicy(context.Background(), &aiconfig.ModelConfig{Provider: "ollama"})
	if err == nil {
		t.Fatal("missing model must fail before provider invocation")
	}
}

func TestWithRequestPolicyUsesTenMinutesWithoutExtendingCallerDeadline(t *testing.T) {
	previous := agentconfig.GetGlobalAIConfig()
	t.Cleanup(func() { agentconfig.SetGlobalAIConfig(previous) })
	agentconfig.SetGlobalAIConfig(&agentconfig.AIConfig{Defaults: agentconfig.AIDefaults{LLM: agentconfig.LLMDefaults{RequestTimeout: 10 * time.Minute}}})
	for _, earlierCaller := range []bool{false, true} {
		ctx := context.Background()
		if earlierCaller {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Minute)
			defer cancel()
		}
		callCtx, mc, cancel, err := withRequestPolicy(ctx, &aiconfig.ModelConfig{Provider: "ollama", Model: "qwen3:8b", Timeout: 5 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if mc.Timeout != 10*time.Minute {
			t.Fatalf("driver timeout=%v, want configured 10m", mc.Timeout)
		}
		deadline, ok := callCtx.Deadline()
		if !ok {
			t.Fatal("missing provider deadline")
		}
		expected := 10 * time.Minute
		if earlierCaller {
			expected = time.Minute
		}
		remaining := time.Until(deadline)
		if remaining > expected || remaining < expected-time.Second {
			t.Fatalf("effective deadline=%s, expected %s", remaining, expected)
		}
		cancel()
	}
}
