package runtime_host

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRealtimeOwnedRedisRetentionCursorAndScope(t *testing.T) {
	endpoint := os.Getenv("X07_OWNED_REDIS_URL")
	if endpoint == "" {
		t.Skip("X07_OWNED_REDIS_REQUIRED")
	}
	options, err := redis.ParseURL(endpoint)
	require.NoError(t, err)
	client := redis.NewClient(options)
	defer client.Close()
	store := NewStore(client, "fixture-x07-"+uuid.NewString())
	ctx := context.Background()
	tenant, subject, namespace := uuid.NewString(), uuid.NewString(), "fixture"
	key := store.realtimeKey(tenant, subject, namespace)
	defer client.Del(ctx, key)
	cursor, err := store.RealtimeCursor(ctx, tenant, subject, namespace)
	require.NoError(t, err)
	require.Equal(t, "0-0", cursor)
	first, err := store.AppendRealtime(ctx, tenant, subject, namespace, []byte(`{"fixture":1}`))
	require.NoError(t, err)
	second, err := store.AppendRealtime(ctx, tenant, subject, namespace, []byte(`{"fixture":2}`))
	require.NoError(t, err)
	rows, err := store.ReadRealtime(ctx, tenant, subject, namespace, first)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, second, rows[0].Cursor)
	for _, scope := range [][3]string{{uuid.NewString(), subject, namespace}, {tenant, uuid.NewString(), namespace}, {tenant, subject, "other"}} {
		cursor, err := store.RealtimeCursor(ctx, scope[0], scope[1], scope[2])
		require.NoError(t, err)
		require.Equal(t, "0-0", cursor)
	}
	other := NewStore(client, "fixture-other-env")
	cursor, err = other.RealtimeCursor(ctx, tenant, subject, namespace)
	require.NoError(t, err)
	require.Equal(t, "0-0", cursor)
	for i := 0; i < RealtimeCapacity; i++ {
		_, err = store.AppendRealtime(ctx, tenant, subject, namespace, []byte(`{"fixture":3}`))
		require.NoError(t, err)
	}
	count, err := client.XLen(ctx, key).Result()
	require.NoError(t, err)
	require.EqualValues(t, RealtimeCapacity, count)
	_, err = store.ReadRealtime(ctx, tenant, subject, namespace, first)
	require.ErrorIs(t, err, ErrRealtimeCursorExpired)
	ttl, err := client.TTL(ctx, key).Result()
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, RealtimeRetention)
	require.NoError(t, client.Del(ctx, key).Err())
	_, err = store.ReadRealtime(ctx, tenant, subject, namespace, second)
	require.ErrorIs(t, err, ErrRealtimeCursorExpired)
	run, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.ReadRealtime(run, tenant, subject, namespace, "0-0")
	require.Error(t, err)
	_, err = store.AppendRealtime(ctx, tenant, subject, namespace, []byte(`not-json`))
	require.Error(t, err)
}
