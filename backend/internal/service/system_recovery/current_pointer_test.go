package system_recovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCurrentRuntimeReadOnlyConfigDirectoryKeepsExistingAnchor(t *testing.T) {
	t.Setenv("POWERX_LINKS_ROOT", t.TempDir())
	r, cfg, source := currentFixture(t)
	before, err := r.StorageRecoveryAnchor().Read(context.Background())
	require.NoError(t, err)
	configDir := filepath.Dir(r.spec.ConfigFile)
	require.NoError(t, os.Chmod(configDir, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(configDir, 0700)) })
	if os.Geteuid() != 0 {
		_, err = os.CreateTemp(configDir, ".snapshot-")
		require.ErrorIs(t, err, os.ErrPermission, "reproduce the systemd /etc write failure")
	}
	restarted, err := NewCurrentRuntime(context.Background(), cfg, r.spec.ConfigFile, source, r.Coordinator.SnapshotStore)
	require.NoError(t, err)
	t.Cleanup(restarted.Close)
	after, err := restarted.StorageRecoveryAnchor().Read(context.Background())
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoFileExists(t, r.spec.ConfigFile+".recovery.json")
	pointer, err := loadCurrentPointer(r.spec.ConfigFile)
	require.NoError(t, err)
	require.Equal(t, r.spec.Directory, pointer["directory"])

	// If configuration replacement is interrupted, the pre-config startup path
	// must still locate the same pending job and independent recovery anchor.
	job, err := r.Coordinator.Store.EnqueueInstanceRestore(privileged(context.Background()), currentReadySnapshot(t, r), uuid.NewString(), 1)
	require.NoError(t, err)
	require.NoError(t, r.beginStorageRestore(context.Background(), &job))
	epoch, err := r.StorageRecoveryAnchor().Read(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.Chmod(configDir, 0700))
	require.NoError(t, os.Rename(r.spec.ConfigFile, r.spec.ConfigFile+".before"))
	t.Setenv("POWERX_CONFIG", r.spec.ConfigFile)
	startup, err := CurrentBeforeConfig()
	require.NoError(t, err)
	require.NotNil(t, startup)
	t.Cleanup(startup.Coordinator.SnapshotStore.Close)
	after, err = startup.StorageRecoveryAnchor().Read(context.Background())
	require.NoError(t, err)
	require.Equal(t, epoch, after)
}

func TestCurrentPointerLegacyCompatibilityAndConflicts(t *testing.T) {
	t.Setenv("POWERX_LINKS_ROOT", "")
	r, _, _ := currentFixture(t)
	legacy := r.spec.ConfigFile + ".recovery.json"
	before, err := os.ReadFile(legacy)
	require.NoError(t, err)
	t.Setenv("POWERX_LINKS_ROOT", t.TempDir())
	pointer, err := loadCurrentPointer(r.spec.ConfigFile)
	require.NoError(t, err)
	require.Equal(t, r.spec.Directory, pointer["directory"])
	require.NoError(t, saveCurrentPointer(r.spec.ConfigFile, r.spec.Directory))
	after, err := os.ReadFile(legacy)
	require.NoError(t, err)
	require.Equal(t, before, after, "preserve the legacy bootstrap pointer")
	file, err := currentPointerFile(r.spec.ConfigFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, []byte("{"), 0600))
	_, err = loadCurrentPointer(r.spec.ConfigFile)
	require.Error(t, err, "a damaged current pointer must not fall back to the legacy copy")
	require.Error(t, saveCurrentPointer(r.spec.ConfigFile, r.spec.Directory))
	require.NoError(t, snapshots.WriteCoordinatorJSON(file, pointer))
	conflicting := map[string]string{"protocol": CurrentRecoveryProtocol, "config_file": r.spec.ConfigFile, "directory": t.TempDir()}
	require.NoError(t, snapshots.WriteCoordinatorJSON(legacy, conflicting))
	_, err = loadCurrentPointer(r.spec.ConfigFile)
	require.Error(t, err)
	require.Error(t, saveCurrentPointer(r.spec.ConfigFile, r.spec.Directory))
}

func TestCurrentPointerRejectsUnsafeDeploymentPaths(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	for _, root := range []string{"relative", "/var/lib/../powerx"} {
		t.Setenv("POWERX_LINKS_ROOT", root)
		_, err := currentPointerFile(configFile)
		require.Error(t, err)
	}
	t.Setenv("POWERX_LINKS_ROOT", t.TempDir())
	file, err := currentPointerFile(configFile)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
	pointer := map[string]string{"protocol": CurrentRecoveryProtocol, "config_file": configFile, "directory": t.TempDir()}
	require.NoError(t, snapshots.WriteCoordinatorJSON(file, pointer))
	require.NoError(t, os.Chmod(file, 0644))
	_, err = loadCurrentPointer(configFile)
	require.Error(t, err)
	require.NoError(t, os.Chmod(file, 0600))
	require.NoError(t, os.Chmod(filepath.Dir(file), 0755))
	_, err = loadCurrentPointer(configFile)
	require.Error(t, err)
	require.NoError(t, os.Chmod(filepath.Dir(file), 0700))
	require.NoError(t, os.Remove(file))
	target := filepath.Join(t.TempDir(), "pointer.json")
	require.NoError(t, snapshots.WriteCoordinatorJSON(target, pointer))
	require.NoError(t, os.Symlink(target, file))
	_, err = loadCurrentPointer(configFile)
	require.Error(t, err)
	require.Error(t, saveCurrentPointer(configFile, pointer["directory"]))
}
