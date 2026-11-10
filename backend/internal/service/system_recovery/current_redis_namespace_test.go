package system_recovery

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/redisnamespace"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCurrentRedisSnapshotAndRestorePreserveOtherInstallation(t *testing.T) {
	binary, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server is required for DUMP/RESTORE validation")
	}
	dir, err := os.MkdirTemp("/tmp", "px-redis-ns-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "redis.sock")
	cmd := exec.Command(binary, "--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no", "--daemonize", "no")
	cmd.Dir = dir
	require.NoError(t, cmd.Start())
	t.Logf("owned Redis pid=%d cwd=%s command=%v TCP ports=none", cmd.Process.Pid, dir, cmd.Args)
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		require.Error(t, cmd.Process.Signal(syscall.Signal(0)))
	})
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	require.Eventually(t, func() bool { return client.Ping(ctx).Err() == nil }, 3*time.Second, 10*time.Millisecond)
	one := redisnamespace.Key("dev:instance-a", "event_fabric:task:one")
	two := redisnamespace.Key("dev:instance-b", "event_fabric:task:one")
	require.NoError(t, client.Set(ctx, one, "before", 0).Err())
	require.NoError(t, client.Set(ctx, two, "other-before", 0).Err())
	image, err := captureCurrentRedisClient(ctx, client, 0, "dev:instance-a")
	require.NoError(t, err)
	require.Len(t, image, 1)
	require.Equal(t, one, image[0].Key)
	require.NoError(t, client.Set(ctx, one, "changed", 0).Err())
	require.NoError(t, client.Set(ctx, two, "other-current", 0).Err())
	require.NoError(t, restoreCurrentRedisClient(ctx, client, 0, image, "dev:instance-a"))
	require.Equal(t, "before", client.Get(ctx, one).Val())
	require.Equal(t, "other-current", client.Get(ctx, two).Val())
	require.Error(t, restoreCurrentRedisClient(ctx, client, 0, image, "dev:instance-b"))
	require.Equal(t, "other-current", client.Get(ctx, two).Val())
}
