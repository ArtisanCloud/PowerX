package system_recovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigurationDoesNotRecursivelyCollectSnapshotStore(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "existing snapshots")
	require.NoError(t, os.MkdirAll(store, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(store, "master.key"), []byte("must not collect"), 0600))
	cfg := &config.Config{SystemSnapshots: config.SnapshotConfig{Directory: store}}
	source := snapshots.Config{ConfigFiles: map[string]string{}, Artifacts: map[string]string{}}
	require.NoError(t, collectRuntimeFiles(cfg, &source, Deployment{Root: root}, root))
	require.Empty(t, source.LifecycleRoots)
	for _, file := range source.ConfigFiles {
		require.NotEqual(t, filepath.Join(store, "master.key"), file)
	}
}
