package llm

import (
	"context"
	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	modelconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPhysicalLeaseCoversConfiguredProviderTimeout(t *testing.T) {
	ttl, err := effectivePhysicalLeaseTTL(6*time.Minute, 10*time.Minute)
	require.NoError(t, err)
	require.Equal(t, 10*time.Minute+10*time.Second, ttl)
	ttl, err = effectivePhysicalLeaseTTL(15*time.Minute, 10*time.Minute)
	require.NoError(t, err)
	require.Equal(t, 15*time.Minute, ttl)
	ttl, err = effectivePhysicalLeaseTTL(6*time.Minute, 5*time.Minute)
	require.NoError(t, err)
	require.Equal(t, 6*time.Minute, ttl)
	for _, tc := range []struct{ lease, request time.Duration }{{0, time.Minute}, {-time.Second, time.Minute}, {2 * time.Hour, time.Minute}, {time.Minute, time.Hour}, {time.Minute, 0}} {
		_, err := effectivePhysicalLeaseTTL(tc.lease, tc.request)
		require.Error(t, err)
	}
}

func TestTenMinuteConfigStartsWithLegacyLeaseOnIsolatedRedis(t *testing.T) {
	binary, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server is required for isolated lease integration")
	}
	// Darwin 的 Unix socket 路径长度有限，不能使用包含完整测试名的 t.TempDir。
	directory, err := os.MkdirTemp("", "px-ai-redis-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	socket := filepath.Join(directory, "r.sock")
	output, err := os.Create(filepath.Join(directory, "redis.log"))
	require.NoError(t, err)
	t.Cleanup(func() { output.Close() })
	cmd := exec.Command(binary, "--port", "0", "--unixsocket", socket, "--unixsocketperm", "600", "--dir", directory, "--appendonly", "yes", "--appendfsync", "always", "--save", "", "--maxmemory-policy", "noeviction")
	cmd.Dir = directory
	cmd.Stdout = output
	cmd.Stderr = output
	require.NoError(t, cmd.Start())
	t.Logf("agent-owned test Redis pid=%d command=%v cwd=%s listener=unix:%s TCP=disabled", cmd.Process.Pid, cmd.Args, directory, socket)
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		connection, err := net.DialTimeout("unix", socket, 100*time.Millisecond)
		if connection != nil {
			connection.Close()
		}
		require.Error(t, err, "test Redis listener must be released")
	})
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
	t.Cleanup(func() { client.Close() })
	require.Eventually(t, func() bool { return client.Ping(context.Background()).Err() == nil }, 3*time.Second, 10*time.Millisecond)
	previousCfg := agentconfig.GetGlobalAIConfig()
	previousPool := configuredPhysicalPool.Load()
	t.Cleanup(func() { agentconfig.SetGlobalAIConfig(previousCfg); configuredPhysicalPool.Store(previousPool) })
	agentconfig.SetGlobalAIConfig(&agentconfig.AIConfig{Defaults: agentconfig.AIDefaults{LLM: agentconfig.LLMDefaults{RequestTimeout: 10 * time.Minute}}, Runtime: agentconfig.AIRuntime{QueueWaitTimeout: time.Minute}})
	rule := agentconfig.PhysicalModelPool{PoolID: "test-timeout-alignment", Provider: "ollama", Endpoint: "http://127.0.0.1:11434", Model: "test-model-no-inference", Capacity: 1, MaxWaiting: 2, LeaseTTL: 6 * time.Minute}
	require.NoError(t, ConfigurePhysicalModelPools(context.Background(), client, []agentconfig.PhysicalModelPool{rule}, time.Minute))
	stored := configuredPhysicalPool.Load().rules[modelTargetKey(rule.Provider, rule.Endpoint, rule.Model)]
	require.Equal(t, 10*time.Minute+10*time.Second, stored.LeaseTTL)
	require.Equal(t, 6*time.Minute, rule.LeaseTTL, "do not mutate the input configuration")
	_, finish, err := acquireModelCallLease(context.Background(), &modelconfig.ModelConfig{Provider: rule.Provider, Endpoint: rule.Endpoint, Model: rule.Model, MaxConcurrentRequests: 1})
	require.NoError(t, err)
	require.NoError(t, finish())
}
