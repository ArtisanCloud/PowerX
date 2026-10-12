package agent_run

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestRecoverRunDispatchesCommittedOutboxAndReleasesDependency(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	queue, _ := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	ctx := context.Background()
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour)}
	run, err := store.Create(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.InstallPlan(ctx, id, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{
		{TaskID: "first"}, {TaskID: "second", DependsOn: []string{"first"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a process exit before any root task is queued/dispatched.
	run, err = store.RecoverRun(ctx, id, queue)
	if err != nil || run.Status != "running" {
		t.Fatalf("recovery did not queue root: %+v %v", run, err)
	}
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker-a", 1, time.Millisecond)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("root absent: %+v %v", deliveries, err)
	}
	// Simulate a Worker exit after persisting the result but before ACK and
	// before queuing the dependent task. The recovery pass must not rerun it.
	run, err = store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
	if err != nil {
		t.Fatal(err)
	}
	_, run, err = store.LeaseTask(ctx, id, run.Version, 1, "first", "worker-a", deliveries[0].Lease.Token, deliveries[0].Lease.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	_, run, err = store.StartTask(ctx, id, run.Version, 1, "first", "worker-a", deliveries[0].Lease.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, run, err = store.CompleteTask(ctx, id, run.Version, 1, "first", "worker-a", deliveries[0].Lease.Token, "result", "evidence")
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.RecoverRun(ctx, id, queue)
	if err != nil || run.Status != "running" {
		t.Fatalf("recovery did not advance: %+v %v", run, err)
	}
	second, err := store.GetTask(ctx, id, 1, "second")
	if err != nil || second.Status != "queued" {
		t.Fatalf("dependency not released: %+v %v", second, err)
	}
	executed := false
	err = store.ProcessDelivery(ctx, queue, deliveries[0], testExecutor(func(context.Context, TaskRef, string) (WorkResult, error) {
		executed = true
		return WorkResult{}, nil
	}))
	if err != nil || executed {
		t.Fatalf("completed side effect was replayed: executed=%t err=%v", executed, err)
	}
	// Repeating the pass cannot create a second outbox attempt.
	_, err = store.RecoverRun(ctx, id, queue)
	if err != nil {
		t.Fatal(err)
	}
	_, _, outboxKey := schedulingKeys(id)
	entries, err := client.XRange(ctx, outboxKey, "-", "+").Result()
	if err != nil || len(entries) != 2 {
		t.Fatalf("duplicate recovered outbox: %+v %v", entries, err)
	}
}
