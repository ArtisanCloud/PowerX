package agent_run

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestModelPoolSharesPhysicalSlotAcrossTenants(t *testing.T) {
	srv := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = clientA.Close(); _ = clientB.Close() })
	poolA, _ := NewRedisModelPool(clientA)
	poolB, _ := NewRedisModelPool(clientB)
	ctx := context.Background()
	first, err := poolA.TryAcquire(ctx, "ollama://local/qwen3:8b", "tenant-a", 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := poolB.TryAcquire(ctx, "ollama://local/qwen3:8b", "tenant-b", 1, time.Second); !errors.Is(err, ErrModelPoolFull) {
		t.Fatalf("second tenant exceeded physical slot: %v", err)
	}
	if _, err := poolB.TryAcquire(ctx, "ollama://local/qwen3:8b", "tenant-b", 2, time.Second); !errors.Is(err, ErrPoolCapacityDrift) {
		t.Fatalf("different per-tenant capacity changed physical limit: %v", err)
	}
	if err := poolA.Release(ctx, first); err != nil {
		t.Fatal(err)
	}
	second, err := poolB.TryAcquire(ctx, "ollama://local/qwen3:8b", "tenant-b", 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if second.Token == first.Token {
		t.Fatal("reused model lease token")
	}
	if err := poolB.Release(ctx, second); err != nil {
		t.Fatal(err)
	}
}

func TestModelPoolRenewAndExpiredLease(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	pool, _ := NewRedisModelPool(client)
	ctx := context.Background()
	lease, err := pool.TryAcquire(ctx, "ollama://local/qwen3:8b", "worker-a", 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	lease, err = pool.Renew(ctx, lease, 2*time.Second)
	if err != nil || !lease.ExpiresAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("model lease renewal failed: %+v %v", lease, err)
	}
	if _, err := pool.TryAcquire(ctx, lease.PoolID, "worker-b", 1, time.Second); !errors.Is(err, ErrModelPoolFull) {
		t.Fatalf("renewed slot was overcommitted: %v", err)
	}
	if err := pool.Release(ctx, lease); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Renew(ctx, lease, time.Second); !errors.Is(err, ErrModelLeaseStale) {
		t.Fatalf("released lease was renewed: %v", err)
	}
}

func TestModelPoolWaitAcquireUsesSharedFIFOAndQueueLimit(t *testing.T) {
	srv := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = clientA.Close(); _ = clientB.Close() })
	poolA, _ := NewRedisModelPool(clientA)
	poolB, _ := NewRedisModelPool(clientB)
	ctx := context.Background()
	id := "ollama://physical/qwen3:8b"
	occupied, err := poolA.TryAcquire(ctx, id, "worker-occupied", 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	type answer struct {
		lease ModelLease
		err   error
	}
	firstResult := make(chan answer, 1)
	secondResult := make(chan answer, 1)
	firstCtx, firstCancel := context.WithTimeout(ctx, 2*time.Second)
	defer firstCancel()
	secondCtx, secondCancel := context.WithTimeout(ctx, 2*time.Second)
	defer secondCancel()
	go func() {
		lease, err := poolA.WaitAcquire(firstCtx, id, "tenant-a", 1, 2, time.Second)
		firstResult <- answer{lease, err}
	}()
	waitForModelQueueSize(t, clientA, id, 1)
	go func() {
		lease, err := poolB.WaitAcquire(secondCtx, id, "tenant-b", 1, 2, time.Second)
		secondResult <- answer{lease, err}
	}()
	waitForModelQueueSize(t, clientA, id, 2)
	if _, err := poolB.WaitAcquire(ctx, id, "tenant-c", 1, 2, time.Second); !errors.Is(err, ErrModelQueueFull) {
		t.Fatalf("unbounded model queue: %v", err)
	}
	if err := poolA.Release(ctx, occupied); err != nil {
		t.Fatal(err)
	}
	if _, err := poolB.TryAcquire(ctx, id, "bypass", 1, time.Second); !errors.Is(err, ErrModelPoolFull) {
		t.Fatalf("immediate acquisition bypassed queued tenants: %v", err)
	}
	var first answer
	select {
	case first = <-firstResult:
	case <-time.After(time.Second):
		t.Fatal("first waiter never acquired")
	}
	if first.err != nil || first.lease.Owner != "tenant-a" {
		t.Fatalf("FIFO first: %+v", first)
	}
	select {
	case unexpected := <-secondResult:
		t.Fatalf("second waiter bypassed first: %+v", unexpected)
	default:
	}
	if err := poolA.Release(ctx, first.lease); err != nil {
		t.Fatal(err)
	}
	select {
	case second := <-secondResult:
		if second.err != nil || second.lease.Owner != "tenant-b" {
			t.Fatalf("FIFO second: %+v", second)
		}
		if err := poolB.Release(ctx, second.lease); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("second waiter never acquired")
	}
}

func waitForModelQueueSize(t *testing.T, client *redis.Client, poolID string, want int64) {
	t.Helper()
	order, _, _ := modelWaitKeys(poolID)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		count, err := client.ZCard(context.Background(), order).Result()
		if err != nil {
			t.Fatal(err)
		}
		if count == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("model wait queue did not reach %d", want)
}
