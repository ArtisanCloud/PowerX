package agent_run

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestReconcilePlanQueuesParallelRootsAndSerialDependency(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	ctx := context.Background()
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour)}
	run, err := store.Create(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.InstallPlan(ctx, id, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{
		{TaskID: "source"}, {TaskID: "campaign"}, {TaskID: "summary", DependsOn: []string{"source", "campaign"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.ReconcilePlan(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, taskID := range []string{"source", "campaign"} {
		task, err := store.GetTask(ctx, id, 1, taskID)
		if err != nil || task.Status != "queued" {
			t.Fatalf("%s: %+v %v", taskID, task, err)
		}
	}
	summary, err := store.GetTask(ctx, id, 1, "summary")
	if err != nil || summary.Status != "pending_dependency" {
		t.Fatalf("summary queued early: %+v %v", summary, err)
	}
	complete := func(taskID string, fence uint64) {
		var err error
		_, run, err = store.LeaseTask(ctx, id, run.Version, 1, taskID, "worker", fence, time.Now().Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		_, run, err = store.StartTask(ctx, id, run.Version, 1, taskID, "worker", fence)
		if err != nil {
			t.Fatal(err)
		}
		_, run, err = store.CompleteTask(ctx, id, run.Version, 1, taskID, "worker", fence, "object://result", "object://evidence")
		if err != nil {
			t.Fatal(err)
		}
	}
	complete("source", 1)
	run, err = store.ReconcilePlan(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	summary, _ = store.GetTask(ctx, id, 1, "summary")
	if summary.Status != "pending_dependency" {
		t.Fatalf("summary queued before campaign: %+v", summary)
	}
	complete("campaign", 2)
	run, err = store.ReconcilePlan(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	summary, _ = store.GetTask(ctx, id, 1, "summary")
	if summary.Status != "queued" {
		t.Fatalf("summary was not queued: %+v", summary)
	}
	complete("summary", 3)
	run, err = store.ReconcilePlan(ctx, id)
	if err != nil || run.Status != "completed" {
		t.Fatalf("run did not finish: %+v %v", run, err)
	}
	_, _, outboxKey := schedulingKeys(id)
	outbox, err := client.XRange(ctx, outboxKey, "-", "+").Result()
	if err != nil || len(outbox) != 3 {
		t.Fatalf("incorrect outbox: %+v %v", outbox, err)
	}
}

func TestReconcilePlanSkipsFailedDependencyAndBlocksManualReview(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	ctx := context.Background()
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour)}
	run, _ := store.Create(ctx, id)
	run, _ = store.InstallPlan(ctx, id, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{
		{TaskID: "first"}, {TaskID: "dependent", DependsOn: []string{"first"}},
	}})
	run, _ = store.ReconcilePlan(ctx, id)
	_, run, _ = store.LeaseTask(ctx, id, run.Version, 1, "first", "worker", 1, time.Now().Add(time.Minute))
	_, run, _ = store.StartTask(ctx, id, run.Version, 1, "first", "worker", 1)
	_, run, _ = store.FailTask(ctx, id, run.Version, 1, "first", "worker", 1, "manual_review_required")
	run, err := store.ReconcilePlan(ctx, id)
	if err != nil || run.Status != "blocked" {
		t.Fatalf("review run did not block: %+v %v", run, err)
	}
	task, err := store.GetTask(ctx, id, 1, "dependent")
	if err != nil || task.Status != "skipped" || task.ReasonCode != "dependency.failed" {
		t.Fatalf("dependency not skipped: %+v %v", task, err)
	}
}

func TestReconcilePlanClassifiesQueueTimeoutBeforeRequestStarts(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisStoreWithQueueWaitTimeout(client, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store.clock = func() time.Time { return now }
	ctx := context.Background()
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: now.Add(time.Hour)}
	run, err := store.Create(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.InstallPlan(ctx, id, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "slow", PoolID: "model:qwen"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ReconcilePlan(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	run, err = store.ReconcilePlan(ctx, id)
	if err != nil || run.Status != "failed" {
		t.Fatalf("queue timeout not terminal: %+v %v", run, err)
	}
	task, err := store.GetTask(ctx, id, 1, "slow")
	if err != nil || task.Status != "failed" || task.ReasonCode != "queue.timeout" || task.QueueWaitMS != 2000 || !task.StartedAt.IsZero() {
		t.Fatalf("queue timeout counted as provider work: %+v %v", task, err)
	}
	events, err := store.Events(ctx, id.TenantUUID, id.Env, id.RunID, 0, 10)
	if err != nil || len(events) != 5 || events[3].QueueWaitMS != 2000 || events[3].PoolID != "model:qwen" {
		t.Fatalf("queue wait missing from trace: %+v %v", events, err)
	}
}

func TestReconcilePlanStopsNewTasksAfterRunDeadline(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	now := time.Now().UTC()
	store.clock = func() time.Time { return now }
	ctx := context.Background()
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: now.Add(time.Minute)}
	run, err := store.Create(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.InstallPlan(ctx, id, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{
		{TaskID: "first"}, {TaskID: "next", DependsOn: []string{"first"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.ReconcilePlan(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, run, err = store.LeaseTask(ctx, id, run.Version, 1, "first", "worker", 1, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_, run, err = store.StartTask(ctx, id, run.Version, 1, "first", "worker", 1)
	if err != nil {
		t.Fatal(err)
	}
	_, run, err = store.CompleteTask(ctx, id, run.Version, 1, "first", "worker", 1, "result", "evidence")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	run, err = store.ReconcilePlan(ctx, id)
	if err != nil || run.Status != "partial" {
		t.Fatalf("late run was not partial: %+v %v", run, err)
	}
	next, err := store.GetTask(ctx, id, 1, "next")
	if err != nil || next.Status != "skipped" || next.ReasonCode != "run.deadline" {
		t.Fatalf("late dependency executed: %+v %v", next, err)
	}
}
