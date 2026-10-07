package agent_run

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestPlanQueueKeepsDependenciesAndOutboxAtomic(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisStore(client)
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
	if validPlan(Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "a", DependsOn: []string{"b"}}, {TaskID: "b", DependsOn: []string{"a"}}}}) {
		t.Fatal("cycle accepted")
	}
	run, err = store.InstallPlan(ctx, identity, run.Version, Plan{
		Revision: 1, Tasks: []TaskDefinition{
			{TaskID: "a", PoolID: "model:qwen"},
			{TaskID: "b", DependsOn: []string{"a"}, PoolID: "model:qwen"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.QueueTask(ctx, identity, run.Version, 1, "b", "dependency")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("dependency was queued: %v", err)
	}
	task, queued, err := store.QueueTask(ctx, identity, run.Version, 1, "a", "model_capacity")
	if err != nil || task.Status != "queued" || task.Attempt != 1 || queued.EventSeq != 3 {
		t.Fatalf("unexpected queue result task=%+v run=%+v err=%v", task, queued, err)
	}
	tasks, err := store.ListTasks(ctx, identity, 1)
	if err != nil || len(tasks) != 2 || tasks[0].TaskID != "a" || tasks[0].Status != "queued" ||
		tasks[1].TaskID != "b" || tasks[1].Status != "pending_dependency" {
		t.Fatalf("snapshot tasks do not match plan: %+v err=%v", tasks, err)
	}
	_, _, err = store.QueueTask(ctx, identity, queued.Version, 1, "a", "duplicate")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate queue was accepted: %v", err)
	}
	_, _, outboxKey := schedulingKeys(identity)
	outbox, err := client.XRange(ctx, outboxKey, "-", "+").Result()
	if err != nil || len(outbox) != 1 || outbox[0].Values["task_id"] != "a" {
		t.Fatalf("outbox does not match queued task: %+v err=%v", outbox, err)
	}
	events, err := store.Events(ctx, identity.TenantUUID, identity.Env, identity.RunID, 0, 10)
	if err != nil || len(events) != 3 {
		t.Fatalf("expected start, plan and queue events: %+v err=%v", events, err)
	}
}

func TestTaskCompletionRejectsExpiredFenceAndReleasesDependency(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisStore(client)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store.clock = func() time.Time { return now }
	ctx := context.Background()
	identity := Snapshot{
		TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: now.Add(time.Hour),
	}
	run, err := store.Create(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.InstallPlan(ctx, identity, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{
		{TaskID: "first"}, {TaskID: "second", DependsOn: []string{"first"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, run, err = store.QueueTask(ctx, identity, run.Version, 1, "first", "")
	if err != nil {
		t.Fatal(err)
	}
	_, run, err = store.LeaseTask(ctx, identity, run.Version, 1, "first", "old", 1, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	_, run, err = store.StartTask(ctx, identity, run.Version, 1, "first", "old", 1)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	_, run, err = store.LeaseTask(ctx, identity, run.Version, 1, "first", "new", 2, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.CompleteTask(ctx, identity, run.Version, 1, "first", "old", 1, "result", "evidence")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expired worker completed task: %v", err)
	}
	_, run, err = store.StartTask(ctx, identity, run.Version, 1, "first", "new", 2)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(500 * time.Millisecond)
	renewed, err := store.RenewTaskLease(ctx, identity, 1, "first", "new", 2, now.Add(2*time.Second))
	if err != nil || !renewed.LeaseUntil.After(now.Add(time.Second)) {
		t.Fatalf("task lease heartbeat failed: %+v %v", renewed, err)
	}
	if _, err := store.RenewTaskLease(ctx, identity, 1, "first", "old", 1, now.Add(2*time.Second)); !errors.Is(err, ErrConflict) {
		t.Fatalf("old worker renewed new fence: %v", err)
	}
	now = now.Add(750 * time.Millisecond)
	_, _, err = store.CompleteTask(ctx, identity, run.Version, 1, "first", "new", 2, "result", "")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("completion without evidence accepted: %v", err)
	}
	_, run, err = store.CompleteTask(ctx, identity, run.Version, 1, "first", "new", 2, "result", "evidence")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.QueueTask(ctx, identity, run.Version, 1, "second", "")
	if err != nil {
		t.Fatalf("completed dependency did not unblock successor: %v", err)
	}
}
