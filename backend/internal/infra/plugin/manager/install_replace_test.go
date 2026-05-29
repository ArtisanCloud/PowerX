package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestInstallFromFileForceReplacesArtifactWithTenantInstances(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	installedRoot := filepath.Join(root, "installed")
	registry := NewJSONRegistry(filepath.Join(root, "registry.json"))
	require.NoError(t, registry.Load(ctx))

	srcV1 := writeReplaceTestPlugin(t, filepath.Join(root, "src-v1"), "first")
	m := &managerImpl{
		opts: Options{
			InstalledRoot: installedRoot,
			Registry:      registry,
			Loader:        NewFSLoader(),
			TenantInstanceCount: func(ctx context.Context, pluginID string) (int64, error) {
				require.Equal(t, "com.powerx.plugins.replace-test", pluginID)
				return 1, nil
			},
		},
	}

	installed, err := m.InstallFromFile(ctx, srcV1, plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	require.Equal(t, "0.1.0", installed.Version)

	srcV2 := writeReplaceTestPlugin(t, filepath.Join(root, "src-v2"), "second")
	replaced, err := m.InstallFromFile(ctx, srcV2, plugin_mgr.InstallOptions{Force: true})
	require.NoError(t, err)
	require.Equal(t, plugin_mgr.StateInstalled, replaced.State)

	got, err := os.ReadFile(filepath.Join(installedRoot, "com.powerx.plugins.replace-test", "0.1.0", "backend", "bin", "plugin"))
	require.NoError(t, err)
	require.Equal(t, "second", string(got))

	err = m.Uninstall(ctx, "com.powerx.plugins.replace-test", "0.1.0")
	require.Error(t, err)
	require.True(t, plugin_mgr.IsCode(err, plugin_mgr.CodeConflict), "err=%v", err)
}

func writeReplaceTestPlugin(t *testing.T, root, binaryContent string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "bin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "web-admin", ".output"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "backend", "bin", "plugin"), []byte(binaryContent), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(`
id: com.powerx.plugins.replace-test
name: Replace Test
version: 0.1.0
runtime:
  kind: process
  entry: backend/bin/plugin
endpoints:
  http_base_path: /api/v1
frontend:
  admin:
    kind: static
    static_dir: web-admin/.output
routes:
  basePath: /api/v1
`), 0o644))
	return root
}
