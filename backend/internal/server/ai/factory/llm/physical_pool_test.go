package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	modelconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPhysicalModelCallWaitsBeforeProviderDeadlineAndReleasesSlot(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	pool, err := agent_run.NewRedisModelPool(client)
	if err != nil {
		t.Fatal(err)
	}
	previous := configuredPhysicalPool.Swap(&physicalPoolRuntime{pool: pool, queueWait: 500 * time.Millisecond,
		rules: map[string]agentconfig.PhysicalModelPool{
			modelTargetKey("ollama", "http://127.0.0.1:11434", "qwen3:8b"): {
				PoolID: "local-qwen", Provider: "ollama", Endpoint: "http://127.0.0.1:11434",
				Model: "qwen3:8b", Capacity: 1, MaxWaiting: 2, LeaseTTL: 2 * time.Second,
			},
		},
	})
	t.Cleanup(func() { configuredPhysicalPool.Store(previous) })
	mc := &modelconfig.ModelConfig{Provider: "ollama", Endpoint: "http://127.0.0.1:11434", Model: "qwen3:8b", MaxConcurrentRequests: 2}
	_, releaseFirst, err := acquireModelCallLease(context.Background(), mc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = releaseFirst() })
	acquired := make(chan struct{}, 1)
	errorsCh := make(chan error, 1)
	go func() {
		callCtx, finish, err := acquireModelCallLease(context.Background(), mc)
		if err != nil {
			errorsCh <- err
			return
		}
		defer finish()
		providerCtx, _, cancel, err := withRequestPolicy(callCtx, mc)
		if err != nil {
			errorsCh <- err
			return
		}
		defer cancel()
		deadline, ok := providerCtx.Deadline()
		if !ok || time.Until(deadline) < RequestTimeout()-time.Second {
			errorsCh <- errors.New("queue waiting consumed provider request timeout")
			return
		}
		acquired <- struct{}{}
	}()
	select {
	case <-acquired:
		t.Fatal("second model call bypassed physical slot")
	case err := <-errorsCh:
		t.Fatal(err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := releaseFirst(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-acquired:
	case err := <-errorsCh:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("waiting model call did not acquire")
	}
}

func TestPhysicalModelCallClassifiesQueueTimeoutAndFailsClosedForUnknownOllama(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	pool, _ := agent_run.NewRedisModelPool(client)
	previous := configuredPhysicalPool.Swap(&physicalPoolRuntime{pool: pool, queueWait: 50 * time.Millisecond,
		rules: map[string]agentconfig.PhysicalModelPool{
			modelTargetKey("ollama", "http://127.0.0.1:11434", "qwen3:8b"): {
				PoolID: "local-qwen", Capacity: 1, MaxWaiting: 2, LeaseTTL: 2 * time.Second,
			},
		},
	})
	t.Cleanup(func() { configuredPhysicalPool.Store(previous) })
	mc := &modelconfig.ModelConfig{Provider: "ollama", Endpoint: "http://127.0.0.1:11434", Model: "qwen3:8b", MaxConcurrentRequests: 2}
	_, release, err := acquireModelCallLease(context.Background(), mc)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, _, err := acquireModelCallLease(context.Background(), mc); !errors.Is(err, ErrModelQueueTimeout) {
		t.Fatalf("physical wait was misclassified: %v", err)
	}
	unknown := *mc
	unknown.Model = "other"
	if _, _, err := acquireModelCallLease(context.Background(), &unknown); !errors.Is(err, ErrPhysicalModelPoolUnconfigured) {
		t.Fatalf("unknown local target bypassed physical pool: %v", err)
	}
}

func TestConfigurePhysicalModelPoolsRequiresDurableRedis(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	previous := configuredPhysicalPool.Load()
	t.Cleanup(func() { configuredPhysicalPool.Store(previous) })
	err := ConfigurePhysicalModelPools(context.Background(), client, []agentconfig.PhysicalModelPool{{
		PoolID: "local-qwen", Provider: "ollama", Endpoint: "http://127.0.0.1:11434",
		Model: "qwen3:8b", Capacity: 1, MaxWaiting: 2, LeaseTTL: 10 * time.Minute,
	}}, time.Minute)
	if err == nil {
		t.Fatal("non-durable Redis passed physical pool startup gate")
	}
}

func TestConfigurePhysicalModelPoolsRealRedis(t *testing.T) {
	addr := os.Getenv("POWERX_AGENT_RUN_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set POWERX_AGENT_RUN_TEST_REDIS_ADDR for a dedicated durable Redis test server")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	previous := configuredPhysicalPool.Load()
	t.Cleanup(func() { configuredPhysicalPool.Store(previous) })
	err := ConfigurePhysicalModelPools(context.Background(), client, []agentconfig.PhysicalModelPool{{
		PoolID: "test-ollama-real", Provider: "ollama", Endpoint: "http://127.0.0.1:11434",
		Model: "qwen3:8b", Capacity: 1, MaxWaiting: 2, LeaseTTL: 6 * time.Minute,
	}}, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
}

func TestInvokeRespectsPhysicalOllamaSlotAcrossParallelCalls(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	pool, _ := agent_run.NewRedisModelPool(client)
	var active atomic.Int32
	var peak atomic.Int32
	started := make(chan struct{}, 2)
	unblockFirst := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		if current > peak.Load() {
			peak.Store(current)
		}
		defer active.Add(-1)
		started <- struct{}{}
		if current == 1 {
			select {
			case <-unblockFirst:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"qwen3:8b","done":true,"message":{"role":"assistant","content":"ok"}}`))
	}))
	defer provider.Close()
	previous := configuredPhysicalPool.Swap(&physicalPoolRuntime{pool: pool, queueWait: time.Second,
		rules: map[string]agentconfig.PhysicalModelPool{
			modelTargetKey("ollama", provider.URL, "qwen3:8b"): {
				PoolID: "test-ollama", Capacity: 1, MaxWaiting: 2, LeaseTTL: 2 * time.Second,
			},
		},
	})
	t.Cleanup(func() { configuredPhysicalPool.Store(previous) })
	mc := &modelconfig.ModelConfig{Provider: "ollama", Endpoint: provider.URL, Model: "qwen3:8b", MaxConcurrentRequests: 2}
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() { _, err := Invoke(context.Background(), mc, "first"); firstDone <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first provider request did not start")
	}
	go func() { _, err := Invoke(context.Background(), mc, "second"); secondDone <- err }()
	select {
	case <-started:
		t.Fatal("second provider request bypassed physical slot")
	case <-time.After(50 * time.Millisecond):
	}
	close(unblockFirst)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("first provider request did not finish")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("queued provider request did not start")
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued provider request did not finish")
	}
	if peak.Load() != 1 {
		t.Fatalf("single-slot Ollama peak concurrency=%d", peak.Load())
	}
}

// 实测真实 Ollama；代理只观测实际请求并控制首个调用起点，不替换模型响应。
func TestInvokeRealOllamaThroughDurablePhysicalPool(t *testing.T) {
	endpoint := os.Getenv("POWERX_TEST_OLLAMA_ENDPOINT")
	redisAddr := os.Getenv("POWERX_AGENT_RUN_TEST_REDIS_ADDR")
	if endpoint == "" || redisAddr == "" {
		t.Skip("set real Ollama and durable Redis endpoints")
	}
	target, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddr})
	t.Cleanup(func() { _ = client.Close() })
	var active, peak atomic.Int32
	firstStarted := make(chan struct{}, 1)
	unblock := make(chan struct{})
	var sequence atomic.Int32
	reverse := httputil.NewSingleHostReverseProxy(target)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		if current > peak.Load() {
			peak.Store(current)
		}
		if sequence.Add(1) == 1 {
			firstStarted <- struct{}{}
			select {
			case <-unblock:
			case <-r.Context().Done():
				return
			}
		}
		reverse.ServeHTTP(w, r)
	}))
	defer provider.Close()
	previous := configuredPhysicalPool.Load()
	t.Cleanup(func() { configuredPhysicalPool.Store(previous) })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	err = ConfigurePhysicalModelPools(ctx, client, []agentconfig.PhysicalModelPool{{PoolID: "test-ollama-real-provider", Provider: "ollama", Endpoint: provider.URL, Model: "qwen3:8b", Capacity: 1, MaxWaiting: 2, LeaseTTL: 6 * time.Minute}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &modelconfig.ModelConfig{Provider: "ollama", Endpoint: provider.URL, Model: "qwen3:8b", MaxConcurrentRequests: 2, MaxTokens: 32, Extra: map[string]any{"think": false}, Timeout: 2 * time.Minute}
	result := make(chan error, 2)
	invoke := func() {
		out, err := Invoke(ctx, cfg, "Reply with OK.")
		if err == nil && (out == nil || out.Text == "") {
			err = errors.New("empty real provider output")
		}
		result <- err
	}
	go invoke()
	select {
	case <-firstStarted:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go invoke()
	select {
	case err := <-result:
		t.Fatalf("second request bypassed held slot: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(unblock)
	for i := 0; i < 2; i++ {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if peak.Load() != 1 || sequence.Load() != 2 {
		t.Fatalf("actual provider calls=%d peak=%d", sequence.Load(), peak.Load())
	}
	t.Logf("real Ollama provider calls=%d peak=%d, profile=2 physical=1", sequence.Load(), peak.Load())
}
