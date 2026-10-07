package agent_run

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCancelFencesStartedTaskAndPreservesUncertainReceipt(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisStore(client)
	require.NoError(t, err)
	queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(time.Minute)}
	run, err := store.Create(ctx, id)
	require.NoError(t, err)
	run, err = store.InstallPlan(ctx, id, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "effect"}, {TaskID: "followup", DependsOn: []string{"effect"}}}})
	require.NoError(t, err)
	_, err = store.ReconcilePlan(ctx, id)
	require.NoError(t, err)
	_, err = store.DispatchPending(ctx, id, queue, 10)
	require.NoError(t, err)
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "cancel-test", "worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- store.ProcessDelivery(ctx, queue, deliveries[0], testExecutor(func(context.Context, TaskRef, string) (WorkResult, error) {
			close(started)
			<-release
			return WorkResult{ResultRef: "object://late", EvidenceRef: "sha256:late"}, nil
		}))
	}()
	<-started
	cancelled, err := store.Cancel(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Status)
	repeated, err := store.Cancel(ctx, id)
	require.NoError(t, err)
	require.Equal(t, cancelled.EventSeq, repeated.EventSeq)
	close(release)
	require.Error(t, <-done)
	task, err := store.GetTask(ctx, id, 1, "effect")
	require.NoError(t, err)
	require.Equal(t, "cancelled", task.Status)
	require.Equal(t, "manual_review_required", task.ReasonCode)
	require.NotEmpty(t, task.ExecutionToken)
	require.Empty(t, task.ResultRef)
	_, err = store.RenewTaskLease(ctx, id, 1, "effect", task.LeaseOwner, task.Fence, time.Now().Add(time.Minute))
	require.ErrorIs(t, err, ErrConflict)
	dependent, err := store.GetTask(ctx, id, 1, "followup")
	require.NoError(t, err)
	require.Equal(t, "cancelled", dependent.Status)
	run, err = store.RecoverRun(ctx, id, queue)
	require.NoError(t, err)
	require.Equal(t, "cancelled", run.Status)
	// Stale delivery is acknowledged without calling the business executor again.
	require.NoError(t, store.ProcessDelivery(ctx, queue, deliveries[0], testExecutor(func(context.Context, TaskRef, string) (WorkResult, error) {
		t.Error("cancelled work executed")
		return WorkResult{}, nil
	})))
}
