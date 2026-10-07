package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	modelconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOllamaSharedPoolProcessHelper(t *testing.T) {
	endpoint := os.Getenv("POWERX_SHARED_PROCESS_ENDPOINT")
	if endpoint == "" {
		t.Skip("subprocess helper")
	}
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("POWERX_SHARED_PROCESS_REDIS")})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	require.NoError(t, ConfigurePhysicalModelPools(ctx, client, []agentconfig.PhysicalModelPool{{PoolID: os.Getenv("POWERX_SHARED_PROCESS_POOL"), Provider: "ollama", Endpoint: endpoint, Model: "qwen3:8b", Capacity: 1, MaxWaiting: 2, LeaseTTL: 6 * time.Minute}}, time.Minute))
	out, err := Invoke(ctx, &modelconfig.ModelConfig{Provider: "ollama", Endpoint: endpoint, Model: "qwen3:8b", MaxConcurrentRequests: 2, MaxTokens: 16, Extra: map[string]any{"think": false}, Timeout: time.Minute}, "Reply with OK.")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotEmpty(t, out.Text)
}

func TestRealOllamaCapacitySharedAcrossRuntimeProcesses(t *testing.T) {
	endpoint, addr := os.Getenv("POWERX_TEST_OLLAMA_ENDPOINT"), os.Getenv("POWERX_AGENT_RUN_TEST_REDIS_ADDR")
	if endpoint == "" || addr == "" {
		t.Skip("set real Ollama and durable Redis endpoints")
	}
	target, err := url.Parse(endpoint)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	poolID := "test-process-" + uuid.NewString()
	sum := sha256.Sum256([]byte(poolID))
	prefix := "agent:model:{" + hex.EncodeToString(sum[:]) + "}"
	defer func() {
		keys := client.Keys(context.Background(), prefix+":*").Val()
		if len(keys) > 0 {
			_ = client.Del(context.Background(), keys...).Err()
		}
	}()
	var active, peak, calls atomic.Int32
	started, unblock := make(chan struct{}, 1), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(unblock) }) }
	defer release()
	reverse := httputil.NewSingleHostReverseProxy(target)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); current > old && !peak.CompareAndSwap(old, current); old = peak.Load() {
		}
		if calls.Add(1) == 1 {
			started <- struct{}{}
			select {
			case <-unblock:
			case <-r.Context().Done():
				return
			}
		}
		reverse.ServeHTTP(w, r)
	}))
	defer func() { release(); provider.Close() }()
	executable, err := os.Executable()
	require.NoError(t, err)
	start := func() (*exec.Cmd, *bytes.Buffer) {
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestOllamaSharedPoolProcessHelper$", "-test.v")
		cmd.Env = append(os.Environ(), "POWERX_SHARED_PROCESS_ENDPOINT="+provider.URL, "POWERX_SHARED_PROCESS_REDIS="+addr, "POWERX_SHARED_PROCESS_POOL="+poolID)
		output := &bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = output, output
		require.NoError(t, cmd.Start())
		return cmd, output
	}
	first, out1 := start()
	defer func() {
		if first.ProcessState == nil {
			_ = first.Process.Kill()
			_ = first.Wait()
		}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	second, out2 := start()
	defer func() {
		if second.ProcessState == nil {
			_ = second.Process.Kill()
			_ = second.Wait()
		}
	}()
	deadline := time.Now().Add(20 * time.Second)
	for client.ZCard(ctx, prefix+":wait_order").Val() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("second runtime process did not queue behind shared slot")
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.EqualValues(t, 1, calls.Load(), "second process must not start provider before obtaining the shared slot")
	release()
	require.NoError(t, first.Wait(), out1.String())
	require.NoError(t, second.Wait(), out2.String())
	require.EqualValues(t, 2, calls.Load())
	require.EqualValues(t, 1, peak.Load())
	t.Log("real runtime processes=2; provider calls=2; shared physical capacity peak=1")
}
