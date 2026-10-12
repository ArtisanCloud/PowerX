package agent_run

import (
	"context"
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerCapacityWaitPreservesQueueTimeAndPlanningIdentity(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { client.Close() })
	store, err := NewRedisStore(client)
	require.NoError(t, err)
	queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "test", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(time.Minute)}
	_, err = store.Create(ctx, id)
	require.NoError(t, err)
	_, err = store.StartPlanning(ctx, id)
	require.NoError(t, err)
	_, err = store.DispatchPending(ctx, id, queue, 10)
	require.NoError(t, err)
	original, err := store.GetTask(ctx, id, 0, PlanningTaskID)
	require.NoError(t, err)
	gate, err := NewExecutionCapacity(ctx, client, "test", ExecutionCapacityPolicy{TenantLimit: 2, RunLimit: 1, LeaseTTL: time.Second})
	require.NoError(t, err)
	blocker, err := gate.Acquire(ctx, id)
	require.NoError(t, err)
	rows, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker-b", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	var ref TaskRef
	require.NoError(t, json.Unmarshal(rows[0].Message.Payload, &ref))
	planned := 0
	worker := &WorkerService{Store: store, Queue: queue, Capacity: gate, Planner: testPlanBuilder(func(context.Context, TaskRef, string) (PlanningResult, error) {
		planned++
		return PlanningResult{Plan: Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "done", PoolID: "workers"}}}, ResultRef: "object://plan", EvidenceRef: "sha256:plan"}, nil
	})}
	require.NoError(t, worker.process(ctx, ref, rows[0]))
	require.Zero(t, planned)
	waiting, err := store.GetTask(ctx, id, 0, PlanningTaskID)
	require.NoError(t, err)
	require.Equal(t, "queued", waiting.Status)
	require.Equal(t, "capacity.run", waiting.QueueReason)
	require.Equal(t, original.QueuedAt, waiting.QueuedAt)
	require.Equal(t, original.Attempt, waiting.Attempt)
	run, err := store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
	require.NoError(t, err)
	seq := run.EventSeq
	_, _, err = store.MarkCapacityWaiting(ctx, id, run.Version, 0, PlanningTaskID, "capacity.run")
	require.ErrorIs(t, err, ErrConflict)
	run, err = store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
	require.NoError(t, err)
	require.Equal(t, seq, run.EventSeq)
	require.NoError(t, gate.Release(ctx, blocker))
	time.Sleep(550 * time.Millisecond)
	rows, err = queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker-a", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NoError(t, worker.process(ctx, ref, rows[0]))
	require.Equal(t, 1, planned)
	complete, err := store.GetTask(ctx, id, 0, PlanningTaskID)
	require.NoError(t, err)
	require.Equal(t, "completed", complete.Status)
	require.Equal(t, original.Attempt, complete.Attempt)
}

type capacityLocator struct{ rows []LocatedRun }

func (l *capacityLocator) ListUnfinishedRuns(_ context.Context, cursor uint64, _ int) ([]LocatedRun, error) {
	var out []LocatedRun
	for _, row := range l.rows {
		if row.Cursor > cursor {
			out = append(out, row)
		}
	}
	return out, nil
}
func (*capacityLocator) FinalizeRun(context.Context, Snapshot) error { return nil }

func TestTwoWorkersShareRunLimitAndAllowOtherTenantProgress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	server := miniredis.RunT(t)
	a := redis.NewClient(&redis.Options{Addr: server.Addr()})
	b := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { a.Close(); b.Close() })
	first, err := NewRedisStore(a)
	require.NoError(t, err)
	second, err := NewRedisStore(b)
	require.NoError(t, err)
	qa, err := event_bus.NewRedisStreamTaskDriver(a, time.Second)
	require.NoError(t, err)
	qb, err := event_bus.NewRedisStreamTaskDriver(b, time.Second)
	require.NoError(t, err)
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "test", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(time.Minute)}
	other := id
	other.TenantUUID = uuid.NewString()
	other.RunID = uuid.NewString()
	other.SessionID = uuid.NewString()
	other.MessageID = uuid.NewString()
	other.TraceID = uuid.NewString()
	_, err = first.Create(ctx, id)
	require.NoError(t, err)
	_, err = first.Create(ctx, other)
	require.NoError(t, err)
	locator := &capacityLocator{rows: []LocatedRun{{Cursor: 1, Identity: id}, {Cursor: 2, Identity: other}}}
	policy := ExecutionCapacityPolicy{TenantLimit: 1, RunLimit: 1, LeaseTTL: time.Second}
	ca, err := NewExecutionCapacity(ctx, a, "test", policy)
	require.NoError(t, err)
	cb, err := NewExecutionCapacity(ctx, b, "test", policy)
	require.NoError(t, err)
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	var active, peak, calls atomic.Int32
	planner := testPlanBuilder(func(_ context.Context, ref TaskRef, _ string) (PlanningResult, error) {
		tasks := []TaskDefinition{{TaskID: "one", PoolID: "workers"}}
		if ref.TenantUUID == id.TenantUUID {
			tasks = append(tasks, TaskDefinition{TaskID: "two", PoolID: "workers"}, TaskDefinition{TaskID: "three", PoolID: "workers"})
		}
		return PlanningResult{Plan: Plan{Revision: 1, Tasks: tasks}, ResultRef: "object://plan", EvidenceRef: "sha256:plan"}, nil
	})
	executor := testExecutor(func(execCtx context.Context, ref TaskRef, _ string) (WorkResult, error) {
		if ref.TenantUUID == id.TenantUUID {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			calls.Add(1)
			select {
			case entered <- struct{}{}:
			default:
			}
			select {
			case <-release:
			case <-execCtx.Done():
				return WorkResult{}, execCtx.Err()
			}
		}
		return WorkResult{ResultRef: "object://result", EvidenceRef: "sha256:result"}, nil
	})
	wa := &WorkerService{Store: first, Queue: qa, Capacity: ca, Planner: planner, Executor: executor, Locator: locator, Concurrency: 4, ScanInterval: time.Second, Owner: "worker-a"}
	wb := &WorkerService{Store: second, Queue: qb, Capacity: cb, Planner: planner, Executor: executor, Locator: locator, Concurrency: 4, ScanInterval: time.Second, Owner: "worker-b"}
	done := make(chan error, 2)
	go func() { done <- wa.Run(ctx) }()
	go func() { done <- wb.Run(ctx) }()
	defer func() {
		cancel()
		for i := 0; i < 2; i++ {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("worker shutdown timed out")
			}
		}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first task did not start")
	}
	require.Eventually(t, func() bool {
		run, e := first.Get(ctx, other.TenantUUID, other.Env, other.RunID)
		if e != nil || run.Status != "completed" {
			return false
		}
		tasks, e := first.ListTasks(ctx, id, 1)
		if e != nil {
			return false
		}
		for _, task := range tasks {
			if task.Status == "queued" && task.QueueReason == "capacity.run" {
				return true
			}
		}
		return false
	}, 3*time.Second, 20*time.Millisecond)
	require.EqualValues(t, 1, calls.Load())
	require.EqualValues(t, 1, peak.Load())
	close(release)
	require.Eventually(t, func() bool {
		run, e := first.Get(ctx, id.TenantUUID, id.Env, id.RunID)
		return e == nil && run.Status == "completed"
	}, 3*time.Second, 20*time.Millisecond)
	require.EqualValues(t, 3, calls.Load())
	require.EqualValues(t, 1, peak.Load())
}

func TestPlanBudgetRejectsOverReservation(t *testing.T) {
	plan := Plan{Revision: 1, Budget: &PlanBudget{MaxSteps: 2, MaxCapabilityCalls: 1}, Tasks: []TaskDefinition{{TaskID: "one", PoolID: "workers", NodeKind: "tooling"}, {TaskID: "two", PoolID: "workers", NodeKind: "tooling"}}}
	require.False(t, ValidPlan(plan))
	plan.Tasks[1].NodeKind = "llm"
	require.True(t, ValidPlan(plan))
	plan.Tasks = append(plan.Tasks, TaskDefinition{TaskID: "three", PoolID: "workers"})
	require.False(t, ValidPlan(plan))
}

func TestWorkerDoesNotStartExpiredQueueBeforeRecoveryScan(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { client.Close() })
	store, err := NewRedisStoreWithQueueWaitTimeout(client, time.Second)
	require.NoError(t, err)
	now := time.Now()
	store.clock = func() time.Time { return now }
	queue, err := event_bus.NewRedisStreamTaskDriver(client, 10*time.Second)
	require.NoError(t, err)
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "test", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: now.Add(time.Minute)}
	_, err = store.Create(ctx, id)
	require.NoError(t, err)
	_, err = store.StartPlanning(ctx, id)
	require.NoError(t, err)
	_, err = store.DispatchPending(ctx, id, queue, 10)
	require.NoError(t, err)
	rows, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	now = now.Add(2 * time.Second)
	var ref TaskRef
	require.NoError(t, json.Unmarshal(rows[0].Message.Payload, &ref))
	calls := 0
	worker := &WorkerService{Store: store, Queue: queue, Planner: testPlanBuilder(func(context.Context, TaskRef, string) (PlanningResult, error) { calls++; return PlanningResult{}, nil })}
	require.NoError(t, worker.process(ctx, ref, rows[0]))
	require.Zero(t, calls)
	task, err := store.GetTask(ctx, id, 0, PlanningTaskID)
	require.NoError(t, err)
	require.Equal(t, "failed", task.Status)
	require.Equal(t, "queue.timeout", task.ReasonCode)
	run, err := store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
	require.NoError(t, err)
	require.Equal(t, "failed", run.Status)
}
