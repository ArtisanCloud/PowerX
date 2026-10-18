package system_recovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/stretchr/testify/require"
)

// 本子进程只在原生集成测试明确给出私有 profile 时运行。
func TestRecoveryCoordinatorCrashChild(t *testing.T) {
	if os.Getenv("POWERX_RECOVERY_CRASH_CHILD") != "1" {
		return
	}
	file := os.Getenv("POWERX_RECOVERY_CRASH_PROFILE")
	profile, e := ReadProfile(file)
	require.NoError(t, e)
	store, e := snapshots.NewCoordinatorStore(profile.Directory, profile.Targets)
	require.NoError(t, e)
	service := snapshots.NewService(context.Background(), snapshots.Config{Directory: profile.SnapshotDirectory}, snapshots.Options{Coordinator: true})
	defer service.Close()
	driver := &Native{Profile: profile, Store: store, SnapshotStore: service}
	coordinator := Coordinator{Store: store, Driver: driver, Initial: profile.Initial, Checkpoint: func(phase string) error {
		if (!profile.InPlace && phase == "pointer_switched") || (profile.InPlace && phase == "instance_replaced") {
			if e := os.WriteFile(os.Getenv("POWERX_RECOVERY_CRASH_SENTINEL"), []byte("owned child reached persisted switch checkpoint"), 0600); e != nil {
				return e
			}
			select {}
		}
		return nil
	}}
	_ = coordinator.Run(context.Background())
}

func crashOwnedCoordinator(t *testing.T, ctx context.Context, root string, profile Profile) {
	t.Helper()
	exe, e := os.Executable()
	require.NoError(t, e)
	sentinel := filepath.Join(root, "owned-crash-checkpoint")
	record := filepath.Join(root, "processes/crash-coordinator.json")
	command, e := startOwned(ctx, record, exe, root, []string{"-test.run=^TestRecoveryCoordinatorCrashChild$", "-test.v"}, cleanEnvironment(root, "POWERX_RECOVERY_CRASH_CHILD=1", "POWERX_RECOVERY_CRASH_PROFILE="+profile.File, "POWERX_RECOVERY_CRASH_SENTINEL="+sentinel), nil)
	require.NoError(t, e)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = stopOwned(cleanup, record)
	})
	require.Eventually(t, func() bool { _, e := os.Stat(sentinel); return e == nil }, 3*time.Minute, 100*time.Millisecond)
	// 只终止本测试刚创建并记录的协调器 PID；Core、源 PG 与候选 PG 分属其自身进程组。
	require.NoError(t, command.Process.Kill())
	require.Eventually(t, func() bool { _, e := processIdentity(command.Process.Pid); return e != nil }, 10*time.Second, 50*time.Millisecond)
	require.NoError(t, stopOwned(ctx, record))
}
