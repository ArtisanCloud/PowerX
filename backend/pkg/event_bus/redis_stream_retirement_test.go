package event_bus

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRetireRunRemovesOnlyItsPendingDelayedAndDeadTasks(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	queue, err := NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	run, other := uuid.NewString(), uuid.NewString()
	makeTask := func(id string, task string) TaskMessage {
		return TaskMessage{ID: id + ":" + task, TenantKey: "tenant:dev", SubscriberID: "agent-run", Topic: "task", Metadata: map[string]string{"run_id": id}}
	}
	dead := makeTask(run, "dead")
	pending := makeTask(run, "pending")
	delayed := makeTask(run, "delayed")
	delayed.VisibleAt = time.Now().Add(time.Hour)
	keep := makeTask(other, "keep")
	for _, m := range []TaskMessage{dead, pending, delayed, keep} {
		require.NoError(t, queue.Enqueue(ctx, m))
	}
	deliveries, err := queue.Dequeue(ctx, "tenant:dev", "agent-run", "workers", "worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	outcome, err := queue.Nack(ctx, deliveries[0], time.Now(), "failed", 1)
	require.NoError(t, err)
	require.Equal(t, StreamDead, outcome)
	at := time.Now().Add(time.Hour)
	require.NoError(t, queue.RetireRun(ctx, "tenant:dev", "agent-run", "workers", run, []string{dead.ID, pending.ID, delayed.ID}, at))
	stream := taskStreamKey("tenant:dev", "agent-run")
	base := strings.TrimSuffix(stream, ":stream")
	require.EqualValues(t, 1, client.XLen(ctx, stream).Val())
	require.EqualValues(t, 0, client.XLen(ctx, base+":dead").Val())
	require.EqualValues(t, 0, client.ZCard(ctx, base+":delay").Val())
	require.NoError(t, queue.Enqueue(ctx, makeTask(run, "late")))
	require.EqualValues(t, 1, client.XLen(ctx, stream).Val(), "late publisher must not revive retired Run")
	// 模拟清理扫描与延迟提升竞争时的旧记录：提升脚本仍须遵守退休标记。
	delayed.VisibleAt = time.Now().Add(-time.Second)
	raw, err := json.Marshal(delayed)
	require.NoError(t, err)
	require.NoError(t, client.ZAdd(ctx, base+":delay", redis.Z{Score: float64(time.Now().Add(-time.Second).UnixMilli()), Member: string(raw)}).Err())
	_, err = queue.PromoteDue(ctx, "tenant:dev", "agent-run", 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, client.XLen(ctx, stream).Val())
	require.NoError(t, queue.RetireRun(ctx, "tenant:dev", "agent-run", "workers", run, []string{dead.ID, pending.ID, delayed.ID}, at))
	deliveries, err = queue.Dequeue(ctx, "tenant:dev", "agent-run", "workers", "worker", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.Equal(t, keep.ID, deliveries[0].Message.ID)
	require.NoError(t, queue.Ack(ctx, deliveries[0]))
}

func TestRetireRunRealRedis(t *testing.T) {
	addr := os.Getenv("POWERX_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set POWERX_TEST_REDIS_ADDR for real Redis retirement")
	}
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	queue, err := NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	tenant, run := "agent-retire-"+uuid.NewString()+":dev", uuid.NewString()
	stream := taskStreamKey(tenant, "agent-run")
	base := strings.TrimSuffix(stream, ":stream")
	t.Cleanup(func() {
		keys := client.Keys(context.Background(), base+":*").Val()
		if len(keys) > 0 {
			_ = client.Del(context.Background(), keys...).Err()
		}
	})
	message := TaskMessage{ID: run + ":task:1", TenantKey: tenant, SubscriberID: "agent-run", Topic: "task", Metadata: map[string]string{"run_id": run}}
	require.NoError(t, queue.Enqueue(ctx, message))
	deliveries, err := queue.Dequeue(ctx, tenant, "agent-run", "workers", "test", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	// 先发布退休标记，模拟与 NACK 竞争；原投递须 ACK 丢弃，不得重新进入延迟队列。
	at := time.Now().Add(time.Hour)
	require.NoError(t, client.ZAdd(ctx, base+":retired_runs", redis.Z{Score: float64(at.UnixMilli()), Member: run}).Err())
	outcome, err := queue.Nack(ctx, deliveries[0], time.Now().Add(time.Minute), "cancelled", 3)
	require.NoError(t, err)
	require.Equal(t, StreamRetired, outcome)
	require.Zero(t, client.ZCard(ctx, base+":delay").Val())
	require.NoError(t, queue.RetireRun(ctx, tenant, "agent-run", "workers", run, []string{message.ID}, at))
	require.NoError(t, queue.Enqueue(ctx, message))
	require.Zero(t, client.XLen(ctx, stream).Val())
}
