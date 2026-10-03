package agent_run

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type workerTestLocator struct {
	identity Snapshot
	writes   atomic.Int32
	done     chan struct{}
}

func (l *workerTestLocator) ListUnfinishedRuns(context.Context, uint64, int) ([]LocatedRun, error) {
	if l.writes.Load() >= 2 {
		return nil, nil
	}
	return []LocatedRun{{Cursor: 1, Identity: l.identity}}, nil
}
func (l *workerTestLocator) FinalizeRun(context.Context, Snapshot) error {
	if l.writes.Add(1) == 1 {
		return errors.New("temporary finalization outage")
	}
	select {
	case l.done <- struct{}{}:
	default:
	}
	return nil
}

func TestWorkerServiceRecoversAcceptedRunAndRetriesFinalization(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisStore(client)
	require.NoError(t, err)
	queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(time.Minute)}
	_, err = store.Create(ctx, id)
	require.NoError(t, err)
	locator := &workerTestLocator{identity: id, done: make(chan struct{}, 1)}
	var planned, executed atomic.Int32
	failures := make(chan error, 10)
	worker := &WorkerService{Store: store, Queue: queue, Locator: locator, Concurrency: 2, ScanInterval: time.Second, Owner: "worker-service-test",
		Planner: testPlanBuilder(func(context.Context, TaskRef, string) (PlanningResult, error) {
			planned.Add(1)
			return PlanningResult{Plan: Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "analysis", PoolID: "workers"}}}, ResultRef: "object://plan", EvidenceRef: "sha256:plan"}, nil
		}),
		Executor: testExecutor(func(context.Context, TaskRef, string) (WorkResult, error) {
			executed.Add(1)
			return WorkResult{ResultRef: "object://analysis", EvidenceRef: "sha256:analysis"}, nil
		}),
		OnError: func(err error) {
			select {
			case failures <- err:
			default:
			}
		},
	}
	stopped := make(chan error, 1)
	go func() { stopped <- worker.Run(ctx) }()
	select {
	case <-locator.done:
	case <-ctx.Done():
		t.Fatal("worker did not finish and recover finalization")
	}
	cancel()
	select {
	case err := <-stopped:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	require.EqualValues(t, 1, planned.Load())
	require.EqualValues(t, 1, executed.Load())
	require.EqualValues(t, 2, locator.writes.Load())
	select {
	case err := <-failures:
		require.ErrorContains(t, err, "temporary finalization outage")
	default:
		t.Fatal("missing finalization failure")
	}
	run, err := store.Get(context.Background(), id.TenantUUID, id.Env, id.RunID)
	require.NoError(t, err)
	require.Equal(t, "completed", run.Status)
}
