package llm

import (
	"context"
	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	modelconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestProfileQueueRespectsConfiguredWaitWithoutPhysicalPool(t *testing.T) {
	previousConfig := agentconfig.GetGlobalAIConfig()
	previousPool := configuredPhysicalPool.Swap(nil)
	t.Cleanup(func() { agentconfig.SetGlobalAIConfig(previousConfig); configuredPhysicalPool.Store(previousPool) })
	agentconfig.SetGlobalAIConfig(&agentconfig.AIConfig{Runtime: agentconfig.AIRuntime{QueueWaitTimeout: 30 * time.Millisecond}})
	mc := &modelconfig.ModelConfig{Provider: "ollama", Model: uuid.NewString(), MaxConcurrentRequests: 1}
	release, err := acquireModelConcurrencyLease(context.Background(), mc)
	require.NoError(t, err)
	defer release()
	// The caller deadline is deliberately much later than queue_wait_timeout.
	// Before the fix this blocked until that parent deadline instead.
	caller, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err = acquireModelCallLease(caller, mc)
	require.ErrorIs(t, err, ErrModelQueueTimeout)
	require.NoError(t, caller.Err())
}

func TestProfileQueueKeepsCallerCancellationDistinct(t *testing.T) {
	previousConfig := agentconfig.GetGlobalAIConfig()
	previousPool := configuredPhysicalPool.Swap(nil)
	t.Cleanup(func() { agentconfig.SetGlobalAIConfig(previousConfig); configuredPhysicalPool.Store(previousPool) })
	agentconfig.SetGlobalAIConfig(&agentconfig.AIConfig{Runtime: agentconfig.AIRuntime{QueueWaitTimeout: time.Minute}})
	mc := &modelconfig.ModelConfig{Provider: "ollama", Model: uuid.NewString(), MaxConcurrentRequests: 1}
	release, err := acquireModelConcurrencyLease(context.Background(), mc)
	require.NoError(t, err)
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = acquireModelCallLease(ctx, mc)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, ErrModelQueueTimeout)
}

func TestPhysicalPoolQueueBudgetAlsoBoundsProfileWait(t *testing.T) {
	previous := configuredPhysicalPool.Swap(&physicalPoolRuntime{queueWait: 30 * time.Millisecond})
	t.Cleanup(func() { configuredPhysicalPool.Store(previous) })
	mc := &modelconfig.ModelConfig{Provider: "ollama", Model: uuid.NewString(), MaxConcurrentRequests: 1}
	release, err := acquireModelConcurrencyLease(context.Background(), mc)
	require.NoError(t, err)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err = acquireModelCallLease(ctx, mc)
	require.ErrorIs(t, err, ErrModelQueueTimeout)
	require.NoError(t, ctx.Err())
}
