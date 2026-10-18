package cache

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

func TestRedisCacheNamespaceSeparatesInstancesAndAllMutations(t *testing.T) {
	server := miniredis.RunT(t)
	port, err := strconv.Atoi(server.Port())
	require.NoError(t, err)
	newStore := func(namespace string) ICache {
		cfg := CacheConfig{Driver: "redis", Host: server.Host(), Port: port, Namespace: namespace}
		store, e := NewCache(&cfg)
		require.NoError(t, e)
		t.Cleanup(func() { _ = store.(*RedisCache).client.Close() })
		return store
	}
	a, b := newStore("instance-A:"), newStore("instance-B:")
	ctx := context.Background()
	require.NoError(t, b.Set(ctx, "counter", 50, 0))
	require.NoError(t, a.Set(ctx, "counter", 1, 0))
	value, err := a.Increment(ctx, "counter", 3)
	require.NoError(t, err)
	require.Equal(t, int64(4), value)
	value, err = a.Decrement(ctx, "counter", 1)
	require.NoError(t, err)
	require.Equal(t, int64(3), value)
	exists, err := a.Exists(ctx, "counter")
	require.NoError(t, err)
	require.True(t, exists)
	require.NoError(t, a.Expire(ctx, "counter", time.Second))
	server.FastForward(2 * time.Second)
	exists, err = a.Exists(ctx, "counter")
	require.NoError(t, err)
	require.False(t, exists)
	require.NoError(t, a.Set(ctx, "counter", 8, 0))
	require.NoError(t, a.Delete(ctx, "counter"))
	raw, err := b.Get(ctx, "counter")
	require.NoError(t, err)
	require.Equal(t, "50", string(raw))
	require.False(t, server.Exists("counter"))
}
