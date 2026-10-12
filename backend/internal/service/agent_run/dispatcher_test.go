package agent_run

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestDispatchPendingReplaysSafelyAcrossQueueBoundary(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisStore(client)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	identity := Snapshot{
		TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour),
	}
	run, err := store.Create(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.InstallPlan(ctx, identity, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "source_analysis"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.QueueTask(ctx, identity, run.Version, 1, "source_analysis", "model_capacity")
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.DispatchPending(ctx, identity, queue, 10)
	if err != nil || n != 1 {
		t.Fatalf("dispatch: %d %v", n, err)
	}
	n, err = store.DispatchPending(ctx, identity, queue, 10)
	if err != nil || n != 0 {
		t.Fatalf("duplicate dispatch: %d %v", n, err)
	}
	// Simulate a crash after enqueue but before the outbox cursor commit.
	_, _, outboxKey := schedulingKeys(identity)
	cursorKey := outboxKey[:len(outboxKey)-len(":outbox")] + ":dispatch_cursor"
	if err := client.Del(ctx, cursorKey).Err(); err != nil {
		t.Fatal(err)
	}
	n, err = store.DispatchPending(ctx, identity, queue, 10)
	if err != nil || n != 1 {
		t.Fatalf("replay dispatch: %d %v", n, err)
	}
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(identity), AgentTaskSubscriber, "workers", "worker-a", 10, time.Millisecond)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("expected one deduplicated delivery: %+v err=%v", deliveries, err)
	}
	var ref TaskRef
	if err := json.Unmarshal(deliveries[0].Message.Payload, &ref); err != nil || ref.RunID != identity.RunID || ref.TaskID != "source_analysis" {
		t.Fatalf("invalid task reference: %+v err=%v", ref, err)
	}
}
