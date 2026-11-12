package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestBootstrapSyncsOnlySelectedManifestAcrossRestarts(t *testing.T) {
	for _, tc := range []struct {
		name         string
		versions     []string
		current      string
		missing      string
		invalid      string
		selectedOnly bool
		want         []string
	}{
		{name: "首次单版本安装", versions: []string{"0.1.11"}, want: []string{"0.1.11"}},
		{name: "新版已启用且保留旧版", versions: []string{"0.1.8", "0.1.10", "0.1.11"}, current: "0.1.11", want: []string{"0.1.11"}},
		{name: "主动选定旧版回滚", versions: []string{"0.1.8", "0.1.10", "0.1.11"}, current: "0.1.10", want: []string{"0.1.10"}},
		{name: "选定但尚未启用", versions: []string{"0.1.8", "0.1.11"}, current: "0.1.11", selectedOnly: true, want: []string{"0.1.11"}},
		{name: "多版本未选定则不猜选", versions: []string{"0.1.8", "0.1.11"}},
		{name: "选定版本缺失不得使用旧版", versions: []string{"0.1.8", "0.1.11"}, current: "0.1.11", missing: "0.1.11"},
		{name: "选定版本无效不得使用旧快照", versions: []string{"0.1.8", "0.1.11"}, current: "0.1.11", invalid: "0.1.11"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			registryPath := filepath.Join(root, "registry.json")
			installedRoot := filepath.Join(root, "installed")
			registry := NewJSONRegistry(registryPath)
			require.NoError(t, registry.Load(ctx))
			const id = "com.example.plugins.bootstrap"
			for _, version := range tc.versions {
				pluginRoot := filepath.Join(installedRoot, id, version)
				writeBootstrapManifestVersion(t, pluginRoot, id, version)
				desc, err := NewFSLoader().LoadDescriptor(ctx, pluginRoot)
				require.NoError(t, err)
				require.NoError(t, registry.Put(ctx, desc, plugin_mgr.StateInstalled))
			}
			if tc.current != "" {
				if tc.selectedOnly {
					require.NoError(t, registry.SetCurrent(ctx, id, tc.current))
				} else {
					require.NoError(t, registry.UpdateState(ctx, id, tc.current, plugin_mgr.StateEnabled))
				}
			}
			require.NoError(t, registry.Save(ctx))
			if tc.missing != "" {
				require.NoError(t, os.RemoveAll(filepath.Join(installedRoot, id, tc.missing)))
			}
			if tc.invalid != "" {
				require.NoError(t, os.WriteFile(filepath.Join(installedRoot, id, tc.invalid, "plugin.yaml"), []byte("runtime: [invalid"), 0o644))
			}

			for restart := 0; restart < 3; restart++ {
				var synced []string
				registry = NewJSONRegistry(registryPath)
				m := &managerImpl{opts: Options{
					Enabled: true, InstalledRoot: installedRoot, Registry: registry, Loader: NewFSLoader(),
					PostInstallManifest: func(_ context.Context, manifest plugin_mgr.Manifest) error {
						synced = append(synced, manifest.Version)
						return nil
					},
				}}
				require.NoError(t, m.Bootstrap(ctx))
				require.Equal(t, tc.want, synced, "restart=%d", restart)
				require.Len(t, registry.ListVersions(ctx, id), len(tc.versions), "保留历史版本登记")
				current, _ := registry.CurrentVersion(ctx, id)
				require.Equal(t, tc.current, current, "启动扫描不得改变版本选择")
			}
		})
	}
}

func writeBootstrapManifestVersion(t *testing.T, root, id, version string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "web-admin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(fmt.Sprintf(`
id: %s
name: Bootstrap version regression
version: %s
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
`, id, version)), 0o644))
}
