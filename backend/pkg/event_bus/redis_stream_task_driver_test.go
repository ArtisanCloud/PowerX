package event_bus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisStreamTaskDriverDedupesAndFencesAcknowledgement(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	driver, err := NewRedisStreamTaskDriver(client, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	message := TaskMessage{ID: "run:task:1", TenantKey: "tenant", SubscriberID: "agent", Topic: "agent.task"}
	if err := driver.Enqueue(ctx, message); err != nil {
		t.Fatal(err)
	}
	if err := driver.Enqueue(ctx, message); err != nil {
		t.Fatalf("duplicate enqueue should be idempotent: %v", err)
	}
	deliveries, err := driver.Dequeue(ctx, "tenant", "agent", "workers", "worker-a", 10, time.Millisecond)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("expected one delivery: %+v err=%v", deliveries, err)
	}
	if err := driver.Ack(ctx, deliveries[0]); err != nil {
		t.Fatal(err)
	}
	if err := driver.Ack(ctx, deliveries[0]); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("duplicate acknowledgement was accepted: %v", err)
	}
	entries, err := client.XLen(ctx, taskStreamKey("tenant", "agent")).Result()
	if err != nil || entries != 0 {
		t.Fatalf("ack did not remove delivery: count=%d err=%v", entries, err)
	}
}

func TestRedisStreamTaskDriverReclaimsAfterLeaseExpiry(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	driver, err := NewRedisStreamTaskDriver(client, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	message := TaskMessage{ID: "run:task:2", TenantKey: "tenant", SubscriberID: "agent", Topic: "agent.task"}
	if err := driver.Enqueue(ctx, message); err != nil {
		t.Fatal(err)
	}
	first, err := driver.Dequeue(ctx, "tenant", "agent", "workers", "worker-a", 1, time.Millisecond)
	if err != nil || len(first) != 1 {
		t.Fatalf("first delivery: %+v err=%v", first, err)
	}
	// miniredis advances key TTL with FastForward; pending-entry idle time uses
	// its wall clock, so both need to pass before XAUTOCLAIM can recover it.
	srv.FastForward(2 * time.Second)
	time.Sleep(1100 * time.Millisecond)
	second, err := driver.Dequeue(ctx, "tenant", "agent", "workers", "worker-b", 1, time.Millisecond)
	if err != nil || len(second) != 1 || second[0].Lease.Token <= first[0].Lease.Token {
		t.Fatalf("reclaim did not advance fence: %+v err=%v", second, err)
	}
	if err := driver.Ack(ctx, first[0]); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("stale worker ack was accepted: %v", err)
	}
	if err := driver.Ack(ctx, second[0]); err != nil {
		t.Fatal(err)
	}
}

func TestRedisStreamTaskRenewKeepsPendingEntryOwned(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	driver, err := NewRedisStreamTaskDriver(client, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := driver.Enqueue(ctx, TaskMessage{ID: "renewed", TenantKey: "tenant", SubscriberID: "agent", Topic: "agent.task"}); err != nil {
		t.Fatal(err)
	}
	first, err := driver.Dequeue(ctx, "tenant", "agent", "workers", "worker-a", 1, time.Millisecond)
	if err != nil || len(first) != 1 {
		t.Fatalf("first delivery: %+v err=%v", first, err)
	}
	time.Sleep(600 * time.Millisecond)
	first[0], err = driver.Renew(ctx, first[0])
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(600 * time.Millisecond)
	other, err := driver.Dequeue(ctx, "tenant", "agent", "workers", "worker-b", 1, time.Millisecond)
	if err != nil || len(other) != 0 {
		t.Fatalf("renewed message was reclaimed: %+v err=%v", other, err)
	}
	if err := driver.Ack(ctx, first[0]); err != nil {
		t.Fatal(err)
	}
}

func TestRedisStreamTaskRetryAndDeadLetter(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	driver, err := NewRedisStreamTaskDriver(client, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := driver.Enqueue(ctx, TaskMessage{ID: "retry", TenantKey: "tenant", SubscriberID: "agent", Topic: "agent.task"}); err != nil {
		t.Fatal(err)
	}
	first, err := driver.Dequeue(ctx, "tenant", "agent", "workers", "worker-a", 1, time.Millisecond)
	if err != nil || len(first) != 1 {
		t.Fatalf("first delivery: %+v err=%v", first, err)
	}
	outcome, err := driver.Nack(ctx, first[0], time.Now().Add(-time.Second), "provider.timeout", 2)
	if err != nil || outcome != StreamRetried {
		t.Fatalf("first failure: %s %v", outcome, err)
	}
	second, err := driver.Dequeue(ctx, "tenant", "agent", "workers", "worker-b", 1, time.Millisecond)
	if err != nil || len(second) != 1 || second[0].Message.Attempt != 1 {
		t.Fatalf("retry delivery: %+v err=%v", second, err)
	}
	if err := driver.Ack(ctx, first[0]); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("old worker ack accepted after NACK: %v", err)
	}
	outcome, err = driver.Nack(ctx, second[0], time.Now(), "provider.timeout", 2)
	if err != nil || outcome != StreamDead {
		t.Fatalf("second failure: %s %v", outcome, err)
	}
	deadKey := taskStreamKey("tenant", "agent")
	deadKey = deadKey[:len(deadKey)-len(":stream")] + ":dead"
	dead, err := client.XRange(ctx, deadKey, "-", "+").Result()
	if err != nil || len(dead) != 1 {
		t.Fatalf("dead-letter entry missing: %+v err=%v", dead, err)
	}
}

func TestRedisStreamTaskDelayedRetryPromotesWhenDue(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	driver, err := NewRedisStreamTaskDriver(client, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := driver.Enqueue(ctx, TaskMessage{ID: "delayed", TenantKey: "tenant", SubscriberID: "agent", Topic: "agent.task"}); err != nil {
		t.Fatal(err)
	}
	first, err := driver.Dequeue(ctx, "tenant", "agent", "workers", "worker-a", 1, time.Millisecond)
	if err != nil || len(first) != 1 {
		t.Fatalf("first delivery: %+v err=%v", first, err)
	}
	outcome, err := driver.Nack(ctx, first[0], time.Now().Add(50*time.Millisecond), "provider.timeout", 3)
	if err != nil || outcome != StreamRetried {
		t.Fatalf("delayed NACK: %s %v", outcome, err)
	}
	n, err := driver.PromoteDue(ctx, "tenant", "agent", 10)
	if err != nil || n != 0 {
		t.Fatalf("promoted early: %d %v", n, err)
	}
	time.Sleep(60 * time.Millisecond)
	n, err = driver.PromoteDue(ctx, "tenant", "agent", 10)
	if err != nil || n != 1 {
		t.Fatalf("did not promote due task: %d %v", n, err)
	}
	second, err := driver.Dequeue(ctx, "tenant", "agent", "workers", "worker-b", 1, time.Millisecond)
	if err != nil || len(second) != 1 || second[0].Message.Attempt != 1 {
		t.Fatalf("promoted delivery: %+v err=%v", second, err)
	}
	if err := driver.Ack(ctx, second[0]); err != nil {
		t.Fatal(err)
	}
}
