package runtime_host

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestHostedCacheNamespacesIsolateIdenticalPluginEntries(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	a, b := NewStore(client, "dev:instance-a"), NewStore(client, "dev:instance-b")
	ctx := context.Background()
	key := Key("same-tenant", "same-plugin", "same-namespace", "same-key")
	require.NoError(t, a.Set(ctx, key, []byte("one"), time.Minute))
	empty, err := b.Get(ctx, key)
	require.NoError(t, err)
	require.False(t, empty.Found)
	require.NoError(t, b.Set(ctx, key, []byte("two"), time.Minute))
	require.NoError(t, a.Delete(ctx, key))
	entry, err := b.Get(ctx, key)
	require.NoError(t, err)
	require.True(t, entry.Found)
	require.Equal(t, "two", string(entry.Value))
}
