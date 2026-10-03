package event_bus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisTaskLeaseRejectsStaleWorker(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	manager, err := NewRedisTaskLeases(client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := manager.Acquire(ctx, "tenant", "agent-worker", "run:task:1", "worker-a", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Acquire(ctx, "tenant", "agent-worker", "run:task:1", "worker-b", time.Second); !errors.Is(err, ErrLeaseBusy) {
		t.Fatalf("second worker acquired live lease: %v", err)
	}
	renewed, err := manager.Renew(ctx, first, 2*time.Second)
	if err != nil || renewed.Token != first.Token {
		t.Fatalf("renew failed: %+v %v", renewed, err)
	}
	srv.FastForward(3 * time.Second)
	if err := manager.Verify(ctx, first); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("expired worker remains valid: %v", err)
	}
	second, err := manager.Acquire(ctx, "tenant", "agent-worker", "run:task:1", "worker-b", time.Second)
	if err != nil || second.Token <= first.Token {
		t.Fatalf("new worker did not receive a higher fence: %+v %v", second, err)
	}
	if err := manager.Release(ctx, first); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("old worker released new lease: %v", err)
	}
	if err := manager.Verify(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := manager.Release(ctx, second); err != nil {
		t.Fatal(err)
	}
}
