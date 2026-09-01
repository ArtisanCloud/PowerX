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

// 页面/API 的 purge 只能删除安装产物。即使内部 destructive 开关被打开，
// 也绝不能经 UninstallAndPurge 调用数据库清理路径。
func TestUninstallAndPurgeNeverCleansDatabaseResources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	installedRoot := filepath.Join(root, "installed")
	pluginID := "com.powerx.plugins.database-safety"
	version := "0.1.0"
	pluginRoot := filepath.Join(installedRoot, pluginID, version)
	require.NoError(t, os.MkdirAll(pluginRoot, 0o755))

	registry := NewJSONRegistry(filepath.Join(root, "registry.json"))
	require.NoError(t, registry.Load(ctx))
	require.NoError(t, registry.Put(ctx, Descriptor{
		Manifest: plugin_mgr.Manifest{ID: pluginID, Version: version},
		Paths:    plugin_mgr.InstalledPaths{Root: pluginRoot},
		// 若数据库清理路径被意外调用，严格绑定校验必须失败；本测试要求
		// purge 仍成功，从而证明页面 purge 不会触碰数据库。
		HostConfig: &plugin_mgr.HostConfig{Spec: map[string]any{"database": map[string]any{}}},
	}, plugin_mgr.StateInstalled))
	require.NoError(t, registry.SetCurrent(ctx, pluginID, version))

	m := &managerImpl{opts: Options{
		InstalledRoot: installedRoot,
		Registry:      registry,
		CoreConfig: &coreconfig.Config{
			Deployment: coreconfig.DeploymentConfig{Env: coreconfig.DeploymentEnvDev},
			Plugin: coreconfig.PluginAggregateConfig{
				PluginConfig: coreconfig.PluginConfig{AllowDestructiveDBCleanup: true},
			},
		},
	}}

	require.NoError(t, m.UninstallAndPurge(ctx, pluginID, version))
	_, err := os.Stat(pluginRoot)
	require.True(t, os.IsNotExist(err), "purge should remove only the plugin artifact directory")
}
