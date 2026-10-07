package agent_run

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// 独占 Redis 子进程与 AOF 目录，故障注入不触及开发共享 Redis。
func TestIsolatedRedisAOFRestartRecoversRunAndPendingDelivery(t *testing.T) {
	binary := os.Getenv("POWERX_TEST_REDIS_SERVER_PATH")
	if binary == "" {
		t.Skip("set local redis-server path for isolated AOF crash drill")
	}
	directory, err := os.MkdirTemp("/tmp", "px-agent-aof-")
	require.NoError(t, err)
	defer os.RemoveAll(directory)
	log, err := os.OpenFile(filepath.Join(directory, "redis.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	require.NoError(t, err)
	defer log.Close()
	socket := filepath.Join(directory, "redis.sock")
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var process *exec.Cmd
	stop := func() {
		if process != nil && process.ProcessState == nil {
			_ = process.Process.Kill()
			_ = process.Wait()
		}
	}
	defer stop()
	start := func() {
		process = exec.Command(binary, "--port", "0", "--unixsocket", socket, "--unixsocketperm", "600", "--dir", directory, "--appendonly", "yes", "--appendfsync", "always", "--save", "", "--maxmemory-policy", "noeviction")
		process.Stdout, process.Stderr = log, log
		require.NoError(t, process.Start())
		for client.Ping(ctx).Err() != nil {
			select {
			case <-ctx.Done():
				t.Fatal("isolated Redis startup timed out")
			case <-time.After(20 * time.Millisecond):
			}
		}
		require.NoError(t, ValidateProductionRedis(ctx, client))
	}
	start()
	store, err := NewRedisStore(client)
	require.NoError(t, err)
	queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(time.Hour)}
	_, err = store.Create(ctx, id)
	require.NoError(t, err)
	_, err = store.StartPlanning(ctx, id)
	require.NoError(t, err)
	_, err = store.DispatchPending(ctx, id, queue, 100)
	require.NoError(t, err)
	deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker-before-crash", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	before, err := store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
	require.NoError(t, err)
	eventsBefore, err := store.Events(ctx, id.TenantUUID, id.Env, id.RunID, 0, 100)
	require.NoError(t, err)
	started := time.Now()
	stop()
	start()
	after, err := store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
	require.NoError(t, err)
	require.Equal(t, before, after, "acknowledged Run must recover from AOF")
	eventsAfter, err := store.Events(ctx, id.TenantUUID, id.Env, id.RunID, 0, 100)
	require.NoError(t, err)
	require.Equal(t, eventsBefore, eventsAfter)
	// 等待旧投递租约及 PEL idle 到期，用另一 Worker 重领同一条消息。
	time.Sleep(1100 * time.Millisecond)
	reclaimed, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker-after-crash", 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, reclaimed, 1)
	require.Equal(t, deliveries[0].Message.ID, reclaimed[0].Message.ID)
	require.Greater(t, reclaimed[0].Lease.Token, deliveries[0].Lease.Token)
	require.ErrorIs(t, queue.Ack(ctx, deliveries[0]), event_bus.ErrLeaseStale)
	builds := 0
	require.NoError(t, store.ProcessPlanningDelivery(ctx, queue, reclaimed[0], testPlanBuilder(func(context.Context, TaskRef, string) (PlanningResult, error) {
		builds++
		return PlanningResult{Plan: Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "source", PoolID: "workers"}, {TaskID: "campaign", PoolID: "workers"}}}, ResultRef: "object://test-plan", EvidenceRef: "object://test-plan-evidence"}, nil
	})))
	require.Equal(t, 1, builds)
	current, err := store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
	require.NoError(t, err)
	require.EqualValues(t, 1, current.PlanRevision)
	t.Logf("isolated AOF appendfsync=always restart recovery=%s, acknowledged Run/events retained, old fence rejected, planning calls=%d", time.Since(started).Round(time.Millisecond), builds)
}
