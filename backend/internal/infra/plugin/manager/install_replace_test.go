package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	coreconfig "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestInstallFromFileForceRejectsEnabledRuntimeWithTenantInstances(t *testing.T) {
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
			CoreConfig: &coreconfig.Config{
				Deployment: coreconfig.DeploymentConfig{Env: coreconfig.DeploymentEnvDev}, EnvironmentID: "existing-environment-test",
			},
			TenantInstanceCount: func(ctx context.Context, pluginID string) (int64, error) {
				require.Equal(t, "com.powerx.plugins.replace-test", pluginID)
				return 1, nil
			},
		},
		databaseSectionBuilder: testDatabaseSection,
	}

	installed, err := m.InstallFromFile(ctx, srcV1, plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	require.Equal(t, "0.1.0", installed.Version)
	require.NoError(t, registry.UpdateState(ctx, installed.ID, installed.Version, plugin_mgr.StateEnabled))

	srcV2 := writeReplaceTestPlugin(t, filepath.Join(root, "src-v2"), "second")
	_, err = m.InstallFromFile(ctx, srcV2, plugin_mgr.InstallOptions{Force: true, ExpectedRevision: installed.Revision})
	require.Error(t, err)
	require.True(t, plugin_mgr.IsCode(err, plugin_mgr.CodeConflict), "err=%v", err)

	got, err := os.ReadFile(filepath.Join(installedRoot, "com.powerx.plugins.replace-test", "0.1.0", "backend", "bin", "plugin"))
	require.NoError(t, err)
	require.Equal(t, "first", string(got))

	err = m.Uninstall(ctx, "com.powerx.plugins.replace-test", "0.1.0")
	require.Error(t, err)
	require.True(t, plugin_mgr.IsCode(err, plugin_mgr.CodeConflict), "err=%v", err)
}

func TestSwitchVersionRejectsEnabledRuntimeWithTenantInstances(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	installedRoot := filepath.Join(root, "installed")
	registry := NewJSONRegistry(filepath.Join(root, "registry.json"))
	require.NoError(t, registry.Load(ctx))

	m := &managerImpl{
		opts: Options{
			InstalledRoot: installedRoot,
			Registry:      registry,
			Loader:        NewFSLoader(),
			CoreConfig: &coreconfig.Config{
				Deployment: coreconfig.DeploymentConfig{Env: coreconfig.DeploymentEnvDev}, EnvironmentID: "existing-environment-test",
			},
			TenantInstanceCount: func(ctx context.Context, pluginID string) (int64, error) {
				require.Equal(t, "com.powerx.plugins.replace-test", pluginID)
				return 1, nil
			},
		},
		databaseSectionBuilder: testDatabaseSection,
	}

	v1 := writeReplaceTestPluginVersion(t, filepath.Join(root, "src-v1"), "0.1.0", "first")
	installed, err := m.InstallFromFile(ctx, v1, plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	require.NoError(t, registry.UpdateState(ctx, installed.ID, installed.Version, plugin_mgr.StateEnabled))

	v2 := writeReplaceTestPluginVersion(t, filepath.Join(root, "src-v2"), "0.2.0", "second")
	// 模拟升级前已存在的历史多版本记录；新安装入口不再允许新增并列版本。
	desc, err := NewFSLoader().LoadDescriptor(ctx, v2)
	require.NoError(t, err)
	desc.HostConfig = installed.HostConfig
	require.NoError(t, registry.Put(ctx, desc, plugin_mgr.StateInstalled))

	_, err = m.SwitchVersion(ctx, installed.ID, "0.2.0", true)
	require.Error(t, err)
	require.True(t, plugin_mgr.IsCode(err, plugin_mgr.CodeConflict), "err=%v", err)
}

func writeReplaceTestPlugin(t *testing.T, root, binaryContent string) string {
	return writeReplaceTestPluginVersion(t, root, "0.1.0", binaryContent)
}

func writeReplaceTestPluginVersion(t *testing.T, root, version, binaryContent string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "bin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "web-admin", ".output"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "backend", "bin", "plugin"), []byte(binaryContent), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(`
id: com.powerx.plugins.replace-test
name: Replace Test
provider:
  id: developer-test
  authority: https://market.example/api/v1
version: `+version+`
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
