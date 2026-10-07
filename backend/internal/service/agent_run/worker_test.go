package agent_run

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type testExecutor func(context.Context, TaskRef, string) (WorkResult, error)

func (f testExecutor) Execute(ctx context.Context, ref TaskRef, key string) (WorkResult, error) {
	return f(ctx, ref, key)
}

func TestWorkerPersistsEvidenceBeforeAcknowledgement(t *testing.T) {
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
	if _, err := store.DispatchPending(ctx, identity, queue, 10); err != nil {
		t.Fatal(err)
	}
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(identity), AgentTaskSubscriber, "workers", "worker-a", 1, time.Millisecond)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("dequeue: %+v %v", deliveries, err)
	}
	executed := 0
	executor := testExecutor(func(_ context.Context, ref TaskRef, key string) (WorkResult, error) {
		executed++
		if ref.RunID != identity.RunID || key != "agent:"+identity.RunID+":1:source_analysis" {
			t.Fatalf("incorrect work identity: %+v %q", ref, key)
		}
		return WorkResult{ResultRef: "object://result", EvidenceRef: "object://evidence"}, nil
	})
	if err := store.ProcessDelivery(ctx, queue, deliveries[0], executor); err != nil {
		t.Fatal(err)
	}
	task, err := store.GetTask(ctx, identity, 1, "source_analysis")
	if err != nil || task.Status != "completed" || task.ResultRef == "" || task.EvidenceRef == "" {
		t.Fatalf("completion not persisted: %+v %v", task, err)
	}
	if executed != 1 {
		t.Fatalf("executor ran %d times", executed)
	}
	finalRun, err := store.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	if err != nil || finalRun.Status != "completed" {
		t.Fatalf("worker did not reconcile final run: %+v %v", finalRun, err)
	}
	if err := queue.Ack(ctx, deliveries[0]); !errors.Is(err, event_bus.ErrLeaseStale) {
		t.Fatalf("worker did not acknowledge after persistence: %v", err)
	}
}

func TestWorkerReleasesSerialDependencyIntoSameRun(t *testing.T) {
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
		{TaskID: "source"}, {TaskID: "summary", DependsOn: []string{"source"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReconcilePlan(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DispatchPending(ctx, id, queue, 10)
	if err != nil {
		t.Fatal(err)
	}
	executor := testExecutor(func(_ context.Context, ref TaskRef, _ string) (WorkResult, error) {
		return WorkResult{ResultRef: "object://" + ref.TaskID, EvidenceRef: "object://evidence"}, nil
	})
	for _, want := range []string{"source", "summary"} {
		deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker-a", 1, time.Millisecond)
		if err != nil || len(deliveries) != 1 {
			t.Fatalf("dequeue %s: %+v %v", want, deliveries, err)
		}
		var ref TaskRef
		if err := json.Unmarshal(deliveries[0].Message.Payload, &ref); err != nil {
			t.Fatal(err)
		}
		if ref.TaskID != want {
			t.Fatalf("wanted %s, got %s", want, ref.TaskID)
		}
		if err := store.ProcessDelivery(ctx, queue, deliveries[0], executor); err != nil {
			t.Fatal(err)
		}
	}
	final, err := store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
	if err != nil || final.Status != "completed" {
		t.Fatalf("serial run did not complete: %+v %v", final, err)
	}
}

func TestWorkerBlocksMissingEvidenceWithoutRetry(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	queue, _ := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	ctx := context.Background()
	identity := Snapshot{
		TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour),
	}
	run, _ := store.Create(ctx, identity)
	run, _ = store.InstallPlan(ctx, identity, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "task"}}})
	_, _, _ = store.QueueTask(ctx, identity, run.Version, 1, "task", "")
	_, _ = store.DispatchPending(ctx, identity, queue, 10)
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(identity), AgentTaskSubscriber, "workers", "worker-a", 1, time.Millisecond)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("dequeue: %+v %v", deliveries, err)
	}
	err = store.ProcessDelivery(ctx, queue, deliveries[0], testExecutor(func(context.Context, TaskRef, string) (WorkResult, error) {
		return WorkResult{ResultRef: "object://result"}, nil
	}))
	if err != nil {
		t.Fatalf("missing evidence was not classified and persisted: %v", err)
	}
	task, err := store.GetTask(ctx, identity, 1, "task")
	if err != nil || task.Status != "failed" || task.ReasonCode != "manual_review_required" {
		t.Fatalf("missing evidence was not blocked for review: %+v %v", task, err)
	}
	if err := queue.Ack(ctx, deliveries[0]); !errors.Is(err, event_bus.ErrLeaseStale) {
		t.Fatalf("review-blocked task was not acknowledged: %v", err)
	}
}

func TestWorkerDoesNotExecuteAfterRunDeadline(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	store.clock = func() time.Time { return time.Now().Add(-time.Hour) }
	queue, _ := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	ctx := context.Background()
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(-time.Minute)}
	run, err := store.Create(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.InstallPlan(ctx, id, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "task"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.QueueTask(ctx, id, run.Version, 1, "task", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DispatchPending(ctx, id, queue, 10)
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker-a", 1, time.Millisecond)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("dequeue: %+v %v", deliveries, err)
	}
	executed := false
	err = store.ProcessDelivery(ctx, queue, deliveries[0], testExecutor(func(context.Context, TaskRef, string) (WorkResult, error) {
		executed = true
		return WorkResult{}, nil
	}))
	if err != nil || executed {
		t.Fatalf("expired run executed side effect: executed=%t err=%v", executed, err)
	}
	task, err := store.GetTask(ctx, id, 1, "task")
	if err != nil || task.Status != "failed" || task.ReasonCode != "run.deadline" {
		t.Fatalf("deadline not classified: %+v %v", task, err)
	}
}
