package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	snapshotops "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
)

// 走真实宿主来源枚举，不启动 Core、不连接数据库或改动开发者的已安装插件。
func TestSystemSnapshotPreflightTracksInstalledPluginAndCompletedBuild(t *testing.T) {
	root := t.TempDir()
	t.Setenv("POWERX_LINKS_ROOT", root)
	catalog := filepath.Join(root, "backend/config/platform_capabilities")
	require.NoError(t, os.MkdirAll(catalog, 0700))
	t.Setenv("PLATFORM_CAPABILITIES_DIR", catalog)
	t.Setenv("POWERX_WEB_OUTPUT", "")
	cfg := config.GetDefaults()
	cfg.Version = "snapshot-preflight-test"
	cfg.Database.Driver = "postgres"
	cfg.Storage.DefaultDriver = "local"
	cfg.Storage.Local.BasePath = filepath.Join(root, "media")
	cfg.Plugin.InstalledDir = filepath.Join(root, "plugins")
	cfg.Plugin.RegistryFile = ""
	require.NoError(t, os.MkdirAll(cfg.Storage.Local.BasePath, 0700))
	source := func(context.Context) (snapshotops.Config, error) { return systemSnapshotSources(cfg) }
	initial, e := source(context.Background())
	require.ErrorContains(t, e, "Web Admin 发布产物缺失")
	s := snapshotops.NewService(context.Background(), initial, snapshotops.Options{Sources: source, Backend: preflightOnlyBackend{}})
	t.Cleanup(s.Close)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 1}), true)
	out, e := s.Overview(ctx)
	require.NoError(t, e)
	require.False(t, out.Ready)
	require.Contains(t, out.Problems, "Web Admin 发布产物缺失，请先完成 npm run build")

	web := filepath.Join(root, "web-admin/.output/server/index.mjs")
	require.NoError(t, os.MkdirAll(filepath.Dir(web), 0700))
	require.NoError(t, os.WriteFile(web, []byte("// built web"), 0600))
	install := func(version string) {
		file := filepath.Join(cfg.Plugin.InstalledDir, "ai-craft", version, "plugin.yaml")
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
		require.NoError(t, os.WriteFile(file, []byte("version: "+version), 0600))
	}
	install("0.1.76")
	out, e = s.Overview(ctx)
	require.NoError(t, e)
	require.True(t, out.Ready, out.Problems)
	require.NoError(t, os.RemoveAll(filepath.Join(cfg.Plugin.InstalledDir, "ai-craft", "0.1.76")))
	install("0.1.77")
	out, e = s.Overview(ctx)
	require.NoError(t, e)
	require.True(t, out.Ready, out.Problems)
	current, e := source(ctx)
	require.NoError(t, e)
	require.Contains(t, current.Artifacts, "plugins/ai-craft/0.1.77/plugin.yaml")
	require.Equal(t, catalog, current.Artifacts["core/platform_capabilities"])
	require.NotContains(t, current.Artifacts, "plugins/ai-craft/0.1.76/plugin.yaml")
}

type preflightOnlyBackend struct{}

func (preflightOnlyBackend) Capture(context.Context, string) (string, error) {
	panic("preflight must not capture")
}
func (preflightOnlyBackend) RestoreDatabase(context.Context, string) (string, string, error) {
	panic("preflight must not restore")
}

func TestSystemSnapshotSourceKeepsResolvedMediaRootBeforeDirectoryIsReady(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("POWERX_LINKS_ROOT", root)
	catalog := filepath.Join(root, "backend/config/platform_capabilities")
	require.NoError(t, os.MkdirAll(catalog, 0700))
	t.Setenv("PLATFORM_CAPABILITIES_DIR", catalog)
	t.Setenv("POWERX_WEB_OUTPUT", "")
	cfg := config.GetDefaults()
	cfg.Database.Driver = "postgres"
	cfg.Storage.DefaultDriver = "local"
	cfg.Storage.Local.BasePath = "./storage/media"
	cfg.Plugin.InstalledDir, cfg.Plugin.RegistryFile = "", ""
	initial, err := systemSnapshotSources(cfg)
	require.ErrorContains(t, err, "媒体根目录必须是存在的普通目录")
	require.True(t, filepath.IsAbs(initial.MediaRoot), "初始化失败不能让已装配的采集器持有原始相对路径")
	require.NoError(t, os.MkdirAll(initial.MediaRoot, 0700))
	web := filepath.Join(root, "web-admin/.output/server/index.mjs")
	require.NoError(t, os.MkdirAll(filepath.Dir(web), 0700))
	require.NoError(t, os.WriteFile(web, []byte("built"), 0600))
	current, err := systemSnapshotSources(cfg)
	require.NoError(t, err)
	require.Equal(t, initial.MediaRoot, current.MediaRoot)
	require.Equal(t, "./storage/media", cfg.Storage.Local.BasePath)
}

func TestSnapshotDirectoryOverrideDoesNotMoveInstallationSources(t *testing.T) {
	root := t.TempDir()
	t.Setenv("POWERX_LINKS_ROOT", root)
	catalog := filepath.Join(root, "backend/config/platform_capabilities")
	require.NoError(t, os.MkdirAll(catalog, 0700))
	t.Setenv("PLATFORM_CAPABILITIES_DIR", catalog)
	t.Setenv("POWERX_WEB_OUTPUT", "")
	cfg := config.GetDefaults()
	cfg.Version = "snapshot-storage-test"
	cfg.Database.Driver = "postgres"
	cfg.Storage.DefaultDriver = "local"
	cfg.Storage.Local.BasePath = t.TempDir()
	cfg.Plugin.InstalledDir = ""
	cfg.Plugin.RegistryFile = ""
	web := filepath.Join(root, "web-admin/.output/server/index.mjs")
	require.NoError(t, os.MkdirAll(filepath.Dir(web), 0700))
	require.NoError(t, os.WriteFile(web, []byte("built"), 0600))
	source, e := systemSnapshotSources(cfg)
	require.NoError(t, e)
	require.Equal(t, filepath.Join(root, "storage/system-snapshots"), source.Directory)
	require.Equal(t, "install_root", source.DirectorySource)
	cfg.SystemSnapshots.Directory = filepath.Join(t.TempDir(), "independent snapshots")
	source, e = systemSnapshotSources(cfg)
	require.NoError(t, e)
	require.Equal(t, cfg.SystemSnapshots.Directory, source.Directory)
	require.Equal(t, "configuration", source.DirectorySource)
	require.Equal(t, filepath.Join(root, "web-admin/.output"), source.Artifacts["core/web-admin"])
	require.Equal(t, cfg.Storage.Local.BasePath, source.MediaRoot)
}
