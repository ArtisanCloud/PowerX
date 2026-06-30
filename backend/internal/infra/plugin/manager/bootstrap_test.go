package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestBootstrapKeepsManagerAvailableWhenPermissionSyncFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	installedRoot := filepath.Join(root, "installed")
	registry := NewJSONRegistry(filepath.Join(root, "registry.json"))
	pluginRoot := filepath.Join(installedRoot, "com.powerx.plugins.bad-permission", "0.1.0")
	writeBootstrapTestPlugin(t, pluginRoot)

	m := &managerImpl{
		opts: Options{
			Enabled:       true,
			InstalledRoot: installedRoot,
			Registry:      registry,
			Loader:        NewFSLoader(),
			PostInstallManifest: func(ctx context.Context, manifest plugin_mgr.Manifest) error {
				return errors.New("permission resource is too long")
			},
		},
	}

	require.NoError(t, m.Bootstrap(ctx))

	items, err := m.List(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "com.powerx.plugins.bad-permission", items[0].ID)
	require.Equal(t, plugin_mgr.StateInstalled, items[0].State)
}

func writeBootstrapTestPlugin(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "web-admin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(`
id: com.powerx.plugins.bad-permission
name: Bad Permission
version: 0.1.0
runtime:
  kind: static
endpoints:
  http_base_path: /api/v1
routes:
  basePath: /api/v1
frontend:
  admin:
    kind: static
    static_dir: web-admin
`), 0o644))
}
