package agent_run

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAgentStateNamespacesSeparateIdenticalRunsInSameEnvironment(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	a, err := NewRedisStoreWithQueueWaitTimeout(client, time.Minute, "dev:instance-a")
	require.NoError(t, err)
	b, err := NewRedisStoreWithQueueWaitTimeout(client, time.Minute, "dev:instance-b")
	require.NoError(t, err)
	initial := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(time.Hour)}
	ctx := context.Background()
	one, err := a.Create(ctx, initial)
	require.NoError(t, err)
	_, err = b.Get(ctx, initial.TenantUUID, initial.Env, initial.RunID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = b.Create(ctx, initial)
	require.NoError(t, err)
	_, err = a.Transition(ctx, initial, one.Version, "running", "agent_run.task_status")
	require.NoError(t, err)
	two, err := b.Get(ctx, initial.TenantUUID, initial.Env, initial.RunID)
	require.NoError(t, err)
	require.Equal(t, "accepted", two.Status)
}
