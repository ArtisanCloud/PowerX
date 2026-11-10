package bootstrap

import (
	"context"
	"net"
	"strconv"
	"testing"

	runtimecache "github.com/ArtisanCloud/PowerX/internal/infra/cache/runtime_host"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
)

func TestRuntimeHostRedisUsesConfiguredCoreCacheWithoutGatewayRedis(t *testing.T) {
	server := miniredis.RunT(t)
	host, port, err := net.SplitHostPort(server.Addr())
	require.NoError(t, err)
	portNumber, err := strconv.Atoi(port)
	require.NoError(t, err)
	store, err := cache.NewCache(&cache.CacheConfig{Driver: "redis", Host: host, Port: portNumber, DB: 3, Namespace: "core-cache-test:"})
	require.NoError(t, err)
	client := runtimeHostRedisClient(store)
	require.NotNil(t, client)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := context.Background()
	require.NoError(t, store.Set(ctx, "ordinary", "cache-value", 0))
	realtime := runtimecache.NewStore(client, "runtime-test-environment")
	cursor, err := realtime.RealtimeCursor(ctx, "tenant", "subject", "namespace")
	require.NoError(t, err)
	require.Equal(t, "0-0", cursor)
	id, err := realtime.AppendRealtime(ctx, "tenant", "subject", "namespace", []byte(`{"event_id":"owned-event"}`))
	require.NoError(t, err)
	require.NotEqual(t, "0-0", id)
	entries, err := realtime.ReadRealtime(ctx, "tenant", "subject", "namespace", "0-0")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	value, err := store.Get(ctx, "ordinary")
	require.NoError(t, err)
	require.Equal(t, "cache-value", string(value))
	other := runtimecache.NewStore(client, "another-runtime-environment")
	cursor, err = other.RealtimeCursor(ctx, "tenant", "subject", "namespace")
	require.NoError(t, err)
	require.Equal(t, "0-0", cursor, "runtime environment namespaces remain isolated")
}

func TestRuntimeHostRedisMissingAndTypedNilStayUnavailable(t *testing.T) {
	require.Nil(t, runtimeHostRedisClient(nil))
	var typedNil *cache.RedisCache
	require.Nil(t, runtimeHostRedisClient(typedNil))
	require.Nil(t, runtimeHostRedisClient(cache.NewRedisCache(nil)))
}
