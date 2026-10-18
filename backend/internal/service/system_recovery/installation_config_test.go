package system_recovery

import (
	"os"
	"path/filepath"
	"testing"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestLegacyLocalCoreUpgradePreservesBusinessAndPinsArtifact(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	id := uuid.NewString()
	instance := filepath.Join(root, id)
	p := Profile{Local: true, File: filepath.Join(root, "profile.json"), Directory: filepath.Join(root, "ops"), Initial: Deployment{ID: id, Root: instance, CoreBinary: filepath.Join(instance, "programs/core/powerx")}}
	require.NoError(t, os.MkdirAll(filepath.Dir(p.Initial.CoreBinary), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(instance, "business"), []byte("test5"), 0600))
	binary := filepath.Join(root, "built-core")
	require.NoError(t, os.WriteFile(binary, []byte("new explicit local build"), 0700))
	require.NoError(t, upgradeLegacyLocalCore(&p, binary, "v1"))
	require.Len(t, p.Overrides, 1)
	require.Equal(t, "core/powerx", p.Overrides[0].Path)
	sha, err := hashFile(p.Overrides[0].Source)
	require.NoError(t, err)
	require.Equal(t, p.Overrides[0].SHA256, sha)
	data, err := os.ReadFile(filepath.Join(instance, "business"))
	require.NoError(t, err)
	require.Equal(t, "test5", string(data))
	// An unfinished restore must not have its program changed underneath it.
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(p.Directory, "active.json"), Active{Maintenance: true, Deployment: p.Initial}))
	require.Error(t, upgradeLegacyLocalCore(&p, binary, "v1"))
}
