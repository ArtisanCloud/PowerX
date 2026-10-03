package agent_run

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type testPlanBuilder func(context.Context, TaskRef, string) (PlanningResult, error)

func (f testPlanBuilder) Build(ctx context.Context, ref TaskRef, key string) (PlanningResult, error) {
	return f(ctx, ref, key)
}

func TestPlanningOutboxSurvivesAdmissionProcessExit(t *testing.T) {
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
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour)}
	_, err = store.Create(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := store.StartPlanning(ctx, id)
	if err != nil || queued.Status != "planning" || queued.EventSeq != 2 {
		t.Fatalf("planning not committed: %+v %v", queued, err)
	}
	tasks, err := store.ListTasks(ctx, id, 0)
	if err != nil || len(tasks) != 1 || tasks[0].TaskID != PlanningTaskID || tasks[0].Status != "queued" {
		t.Fatalf("planning task absent from snapshot: %+v %v", tasks, err)
	}
	repeat, err := store.StartPlanning(ctx, id)
	if err != nil || repeat.Version != queued.Version {
		t.Fatalf("duplicate planning request: %+v %v", repeat, err)
	}
	// The API process exits before publishing to TaskBus. Recovery reads the
	// committed outbox and dispatches the same stable planning message.
	recovered, err := store.RecoverRun(ctx, id, queue)
	if err != nil || recovered.Version != queued.Version {
		t.Fatalf("planning recovery: %+v %v", recovered, err)
	}
	_, err = store.RecoverRun(ctx, id, queue)
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "planning", "worker-a", 10, time.Millisecond)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("expected one planning delivery: %+v %v", deliveries, err)
	}
	var ref TaskRef
	if err := json.Unmarshal(deliveries[0].Message.Payload, &ref); err != nil {
		t.Fatal(err)
	}
	if ref.RunID != id.RunID || ref.Revision != 0 || ref.TaskID != PlanningTaskID || ref.Attempt != 1 {
		t.Fatalf("invalid planning ref: %+v", ref)
	}
	events, err := store.Events(ctx, id.TenantUUID, id.Env, id.RunID, 0, 10)
	if err != nil || len(events) != 2 || events[1].Type != "agent_run.planning_queued" {
		t.Fatalf("planning event duplicated or missing: %+v %v", events, err)
	}
}

func TestRecoveryStartsPlanningAfterAcceptedAnchorCrash(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	queue, _ := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	ctx := context.Background()
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour)}
	_, err := store.Create(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		run, err := store.RecoverRun(ctx, id, queue)
		if err != nil || run.Status != "planning" || run.EventSeq != 2 {
			t.Fatalf("accepted anchor recovery %d: %+v %v", i, run, err)
		}
	}
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "planning", "worker-a", 10, time.Millisecond)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("planning message missing or duplicated: %+v %v", deliveries, err)
	}
}

func TestRecoveryExpiresAcceptedAnchorWithoutStartingPlanner(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	queue, _ := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	now := time.Now().UTC()
	store.clock = func() time.Time { return now }
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: now.Add(time.Second)}
	ctx := context.Background()
	_, err := store.Create(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	run, err := store.RecoverRun(ctx, id, queue)
	if err != nil || run.Status != "failed" {
		t.Fatalf("expired accepted run: %+v %v", run, err)
	}
	events, err := store.Events(ctx, id.TenantUUID, id.Env, id.RunID, 1, 10)
	if err != nil || len(events) != 1 || events[0].ReasonCode != "run.deadline" {
		t.Fatalf("accepted deadline reason missing: %+v %v", events, err)
	}
}

func TestPlanningCompletionFencesOldWorkerAndRecoversRoots(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	queue, _ := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	ctx := context.Background()
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour)}
	_, err := store.Create(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartPlanning(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DispatchPending(ctx, id, queue, 10)
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "planning", "worker-a", 1, time.Millisecond)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("planning delivery: %+v %v", deliveries, err)
	}
	lease := deliveries[0].Lease
	_, run, err = store.LeaseTask(ctx, id, run.Version, 0, PlanningTaskID, lease.Owner, lease.Token, lease.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	_, run, err = store.StartTask(ctx, id, run.Version, 0, PlanningTaskID, lease.Owner, lease.Token)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "source"}, {TaskID: "summary", DependsOn: []string{"source"}}}}
	if _, err := store.CompletePlanning(ctx, id, run.Version, lease.Owner, lease.Token+1, plan, "object://plan", "object://evidence"); err != ErrConflict {
		t.Fatalf("stale fence committed plan: %v", err)
	}
	completed, err := store.CompletePlanning(ctx, id, run.Version, lease.Owner, lease.Token, plan, "object://plan", "object://evidence")
	if err != nil || completed.PlanRevision != 1 {
		t.Fatalf("plan was not committed: %+v %v", completed, err)
	}
	if _, err := store.CompletePlanning(ctx, id, run.Version, lease.Owner, lease.Token, plan, "object://plan", "object://evidence"); err != ErrConflict {
		t.Fatalf("duplicate completion installed plan: %v", err)
	}
	// Crash before ACK and root dispatch. Recovery queues only the ready root.
	recovered, err := store.RecoverRun(ctx, id, queue)
	if err != nil || recovered.PlanRevision != 1 {
		t.Fatalf("recover planned run: %+v %v", recovered, err)
	}
	source, err := store.GetTask(ctx, id, 1, "source")
	if err != nil || source.Status != "queued" {
		t.Fatalf("root not queued: %+v %v", source, err)
	}
	summary, err := store.GetTask(ctx, id, 1, "summary")
	if err != nil || summary.Status != "pending_dependency" {
		t.Fatalf("dependent task ran early: %+v %v", summary, err)
	}
	planningTask, err := store.GetTask(ctx, id, 0, PlanningTaskID)
	if err != nil || planningTask.Status != "completed" {
		t.Fatalf("planning receipt missing: %+v %v", planningTask, err)
	}
	if err := store.ProcessPlanningDelivery(ctx, queue, deliveries[0], testPlanBuilder(func(context.Context, TaskRef, string) (PlanningResult, error) {
		t.Fatal("completed planning task was rebuilt")
		return PlanningResult{}, nil
	})); err != nil {
		t.Fatalf("duplicate planning delivery did not ACK safely: %v", err)
	}
}

func TestPlanningWorkerPublishesParallelRootsOnce(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	queue, _ := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	ctx := context.Background()
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour)}
	_, err := store.Create(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.StartPlanning(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DispatchPending(ctx, id, queue, 10)
	if err != nil {
		t.Fatal(err)
	}
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker-a", 1, time.Millisecond)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("planning delivery: %+v %v", deliveries, err)
	}
	builds := 0
	builder := testPlanBuilder(func(_ context.Context, ref TaskRef, key string) (PlanningResult, error) {
		builds++
		if ref.RunID != id.RunID || key != "agent:"+id.RunID+":plan:1" {
			t.Fatalf("planning identity mismatch: %+v %s", ref, key)
		}
		return PlanningResult{Plan: Plan{Revision: 1, Tasks: []TaskDefinition{
			{TaskID: "source"}, {TaskID: "campaign"}, {TaskID: "summary", DependsOn: []string{"source", "campaign"}},
		}}, ResultRef: "object://plan", EvidenceRef: "object://plan-evidence"}, nil
	})
	if err := store.ProcessPlanningDelivery(ctx, queue, deliveries[0], builder); err != nil {
		t.Fatal(err)
	}
	if builds != 1 {
		t.Fatalf("planner ran %d times", builds)
	}
	for _, taskID := range []string{"source", "campaign"} {
		task, err := store.GetTask(ctx, id, 1, taskID)
		if err != nil || task.Status != "queued" {
			t.Fatalf("independent root %s not queued: %+v %v", taskID, task, err)
		}
	}
	summary, err := store.GetTask(ctx, id, 1, "summary")
	if err != nil || summary.Status != "pending_dependency" {
		t.Fatalf("dependent task queued too early: %+v %v", summary, err)
	}
	ready, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker-b", 10, time.Millisecond)
	if err != nil || len(ready) != 2 {
		t.Fatalf("independent roots were not dispatched: %+v %v", ready, err)
	}
}

func TestPlanningRecoveryClassifiesQueueAndRunTimeout(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		deadline     time.Duration
	}{
		{name: "queue_wait", reason: "queue.timeout", deadline: time.Hour},
		{name: "run_deadline", reason: "run.deadline", deadline: time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			store, _ := NewRedisStoreWithQueueWaitTimeout(client, time.Second)
			queue, _ := event_bus.NewRedisStreamTaskDriver(client, time.Second)
			now := time.Now().UTC()
			store.clock = func() time.Time { return now }
			id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
				SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
				Status: "accepted", DeadlineAt: now.Add(tc.deadline)}
			ctx := context.Background()
			_, err := store.Create(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.StartPlanning(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			now = now.Add(2 * time.Second)
			run, err := store.RecoverRun(ctx, id, queue)
			if err != nil || run.Status != "failed" {
				t.Fatalf("planning timeout: %+v %v", run, err)
			}
			task, err := store.GetTask(ctx, id, 0, PlanningTaskID)
			if err != nil || task.ReasonCode != tc.reason {
				t.Fatalf("incorrect timeout reason: %+v %v", task, err)
			}
			events, err := store.Events(ctx, id.TenantUUID, id.Env, id.RunID, 2, 10)
			if err != nil || len(events) != 1 || events[0].ReasonCode != tc.reason {
				t.Fatalf("timeout event missing reason: %+v %v", events, err)
			}
		})
	}
}

func TestPlanningFailureIsPersistedWithoutRepeatedModelCalls(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(fmt.Sprint(panics), func(t *testing.T) {
			ctx := context.Background()
			client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
			defer client.Close()
			store, _ := NewRedisStore(client)
			queue, _ := event_bus.NewRedisStreamTaskDriver(client, time.Second)
			id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(time.Minute)}
			if _, err := store.Create(ctx, id); err != nil {
				t.Fatal(err)
			}
			if _, err := store.RecoverRun(ctx, id, queue); err != nil {
				t.Fatal(err)
			}
			deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "test-failure", "worker", 1, time.Millisecond)
			if err != nil || len(deliveries) != 1 {
				t.Fatalf("delivery: %v", err)
			}
			calls := 0
			builder := testPlanBuilder(func(context.Context, TaskRef, string) (PlanningResult, error) {
				calls++
				if panics {
					panic("invalid plan")
				}
				return PlanningResult{}, fmt.Errorf("invalid plan")
			})
			if err := store.ProcessPlanningDelivery(ctx, queue, deliveries[0], builder); err != nil {
				t.Fatal(err)
			}
			run, err := store.RecoverRun(ctx, id, queue)
			if err != nil || run.Status != "failed" || calls != 1 {
				t.Fatalf("planner failure not terminal: %+v calls=%d err=%v", run, calls, err)
			}
			task, err := store.GetTask(ctx, id, 0, PlanningTaskID)
			if err != nil || task.ReasonCode != "planner.failed" {
				t.Fatalf("missing failure reason: %+v %v", task, err)
			}
		})
	}
}
