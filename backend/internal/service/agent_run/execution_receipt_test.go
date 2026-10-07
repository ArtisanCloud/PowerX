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
	"github.com/stretchr/testify/require"
)

type receiptTestQueue struct{ acked int }

func (*receiptTestQueue) Enqueue(context.Context, event_bus.TaskMessage) error { return nil }
func (*receiptTestQueue) Renew(_ context.Context, d event_bus.StreamTaskDelivery) (event_bus.StreamTaskDelivery, error) {
	return d, nil
}
func (q *receiptTestQueue) Ack(context.Context, event_bus.StreamTaskDelivery) error {
	q.acked++
	return nil
}

func TestWorkerRecoveryNeverRepeatsUncertainBusinessExecution(t *testing.T) {
	for _, saved := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncertain", true: "receipt_saved"}[saved], func(t *testing.T) {
			ctx := context.Background()
			srv := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			store, err := NewRedisStore(client)
			require.NoError(t, err)
			now := time.Now()
			store.clock = func() time.Time { return now }
			id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: now.Add(time.Hour)}
			run, err := store.Create(ctx, id)
			require.NoError(t, err)
			run, err = store.InstallPlan(ctx, id, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "write"}}})
			require.NoError(t, err)
			_, run, err = store.QueueTask(ctx, id, run.Version, 1, "write", "")
			require.NoError(t, err)
			_, run, err = store.LeaseTask(ctx, id, run.Version, 1, "write", "old", 1, now.Add(time.Second))
			require.NoError(t, err)
			_, _, err = store.StartTask(ctx, id, run.Version, 1, "write", "old", 1)
			require.NoError(t, err)
			ref := TaskRef{TenantUUID: id.TenantUUID, Env: id.Env, RunID: id.RunID, Revision: 1, TaskID: "write", Attempt: 1}
			token, _, err := store.executionReceipt(ctx, ref, "old", 1, "", nil)
			require.NoError(t, err)
			require.NotEmpty(t, token)
			_, _, err = store.executionReceipt(ctx, ref, "old", 1, "", nil)
			require.ErrorIs(t, err, ErrExecutionUncertain)
			result := WorkResult{ResultRef: "object://result", EvidenceRef: "object://evidence"}
			if saved {
				_, _, err = store.executionReceipt(ctx, ref, "old", 1, token, &result)
				require.NoError(t, err)
			}
			// 模拟旧 Worker 退出，TaskBus 用更高 fence 重领。
			now = now.Add(2 * time.Second)
			payload, err := json.Marshal(ref)
			require.NoError(t, err)
			d := event_bus.StreamTaskDelivery{Message: event_bus.TaskMessage{ID: id.RunID + ":1:write:1", TenantKey: taskQueueTenant(id), SubscriberID: AgentTaskSubscriber, Payload: payload}}
			d.Lease.MessageID, d.Lease.Owner, d.Lease.Token, d.Lease.ExpiresAt = d.Message.ID, "new", 2, now.Add(time.Minute)
			queue := &receiptTestQueue{}
			calls := 0
			err = store.ProcessDelivery(ctx, queue, d, testExecutor(func(context.Context, TaskRef, string) (WorkResult, error) { calls++; return result, nil }))
			require.NoError(t, err)
			require.Zero(t, calls)
			require.Equal(t, 1, queue.acked)
			final, err := store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
			require.NoError(t, err)
			if saved {
				require.Equal(t, "completed", final.Status)
			} else {
				require.Equal(t, "blocked", final.Status)
			}
			_, _, err = store.executionReceipt(ctx, ref, "old", 1, token, &result)
			require.ErrorIs(t, err, ErrConflict)
		})
	}
}
