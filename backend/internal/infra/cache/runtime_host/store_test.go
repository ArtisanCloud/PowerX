package runtime_host

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestTypedNilDependencyFails(t *testing.T) {
	var client *redis.Client
	s := NewStore(client)
	_, e := s.Get(context.Background(), "key")
	require.Error(t, e)
	require.Error(t, s.Set(context.Background(), "key", nil, time.Second))
	require.Error(t, s.Delete(context.Background(), "key"))
}

func TestRealRedisAtomicExpiry(t *testing.T) {
	if os.Getenv("POWERX_RUNTIME_HOST_TEST_REDIS") != "1" {
		t.Skip("POWERX_RUNTIME_HOST_TEST_REDIS=1")
	}
	binary, e := exec.LookPath("redis-server")
	require.NoError(t, e)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, e)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	cmd := exec.Command(binary, "--bind", "127.0.0.1", "--port", strconv.Itoa(port), "--save", "", "--appendonly", "no", "--dir", t.TempDir())
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	client := redis.NewClient(&redis.Options{Addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), MaxRetries: -1})
	t.Cleanup(func() { client.Close() })
	ctx := context.Background()
	require.Eventually(t, func() bool { return client.Ping(ctx).Err() == nil }, 5*time.Second, 20*time.Millisecond)
	s := NewStore(client)
	key := Key(uuid.NewString(), uuid.NewString(), "n", "k")
	require.NoError(t, s.Set(ctx, key, []byte{}, 200*time.Millisecond))
	entry, e := s.Get(ctx, key)
	require.NoError(t, e)
	require.True(t, entry.Found)
	require.Empty(t, entry.Value)
	ttl, e := client.PTTL(ctx, key).Result()
	require.NoError(t, e)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, 200*time.Millisecond)
	expiry := entry.ExpiresAt
	entry, e = s.Get(ctx, key)
	require.NoError(t, e)
	require.Equal(t, expiry, entry.ExpiresAt)
	require.Eventually(t, func() bool { entry, e := s.Get(ctx, key); return e == nil && !entry.Found }, time.Second, 20*time.Millisecond)
	require.NoError(t, s.Set(ctx, key, []byte{0, 255, 10}, time.Second))
	entry, e = s.Get(ctx, key)
	require.NoError(t, e)
	require.Equal(t, []byte{0, 255, 10}, entry.Value)
	require.NoError(t, client.Persist(ctx, key).Err())
	_, e = s.Get(ctx, key)
	require.Error(t, e)
	require.NoError(t, s.Delete(ctx, key))
	require.NoError(t, s.Delete(ctx, key))
	require.NotEqual(t, Key("a", "b", "c:d", "e"), Key("a", "b", "c", "d:e"))
	require.NotEqual(t, Key("a", "b", "c", "d"), Key("a", "other", "c", "d"))
}
