package manager

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestInstallOriginSurvivesRegistryReloadAndBootstrap(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	installedRoot := filepath.Join(root, "installed")
	pluginRoot := filepath.Join(installedRoot, "com.powerx.plugins.bad-permission", "0.1.0")
	writeBootstrapTestPlugin(t, pluginRoot)
	desc, err := NewFSLoader().LoadDescriptor(ctx, pluginRoot)
	require.NoError(t, err)
	origin := plugin_mgr.InstallOrigin{Type: "marketplace", SourceID: "https://market.example/api/v1", SHA256: "verified-artifact-hash", Channel: "catalog"}
	desc.InstallOrigin = origin
	regPath := filepath.Join(root, "registry.json")
	reg := NewJSONRegistry(regPath)
	require.NoError(t, reg.Put(ctx, desc, plugin_mgr.StateDisabled))
	require.NoError(t, reg.Save(ctx))
	reloaded := NewJSONRegistry(regPath)
	mgr := &managerImpl{opts: Options{InstalledRoot: installedRoot, Registry: reloaded, Loader: NewFSLoader()}}
	require.NoError(t, mgr.Bootstrap(ctx))
	got, ok := reloaded.GetVersion(ctx, desc.Manifest.ID, desc.Manifest.Version)
	require.True(t, ok)
	require.Equal(t, origin, got.InstallOrigin)
	require.Equal(t, plugin_mgr.StateDisabled, got.State)
	// 未记录来源的旧版本不能因为启动时从磁盘发现而被归类为本地。
	desc.Manifest.Version = "0.0.1"
	desc.InstallOrigin = plugin_mgr.InstallOrigin{}
	require.NoError(t, reloaded.Put(ctx, desc, plugin_mgr.StateInstalled))
	legacy, ok := reloaded.GetVersion(ctx, desc.Manifest.ID, "0.0.1")
	require.True(t, ok)
	require.Equal(t, "unknown", legacy.InstallOrigin.Type)
}
