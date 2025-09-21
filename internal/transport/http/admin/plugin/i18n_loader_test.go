package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestLoadPluginMenuI18n(t *testing.T) {
	dir := t.TempDir()
	adminI18n := filepath.Join(dir, "frontend", "admin", "i18n")
	require.NoError(t, os.MkdirAll(filepath.Join(adminI18n, "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(adminI18n, "zh-CN"), 0o755))

	enFile := filepath.Join(adminI18n, "en", "menus.json")
	zhFile := filepath.Join(adminI18n, "zh-CN", "menus.yaml")

	require.NoError(t, os.WriteFile(enFile, []byte(`{"hello":"Hello"}`), 0o644))
	require.NoError(t, os.WriteFile(zhFile, []byte("greeting: \"你好\"\n"), 0o644))

	plugin := plugin_mgr.Plugin{
		ID: "com.example.demo",
		Frontend: plugin_mgr.FrontendSpec{
			Admin: plugin_mgr.FrontendAdminSpec{
				I18n: &plugin_mgr.I18nSpec{
					Dir:              "./frontend/admin/i18n",
					DefaultNamespace: "menus",
				},
			},
		},
		Paths: plugin_mgr.InstalledPaths{
			FrontendAdminI18nDir: adminI18n,
		},
	}

	bundle := loadPluginMenuI18n(context.Background(), plugin, nil)
	require.NotNil(t, bundle)
	require.Equal(t, "com.example.demo", bundle.PluginID)
	require.Equal(t, "i18next", bundle.Format)
	require.ElementsMatch(t, []string{"menus"}, bundle.Namespaces)
	require.Contains(t, bundle.Locales, "en")
	require.Contains(t, bundle.Locales, "zh-CN")

	enNs, ok := bundle.Locales["en"]["menus"]
	require.True(t, ok)
	require.Equal(t, "Hello", enNs["hello"])

	zhNs, ok := bundle.Locales["zh-CN"]["menus"]
	require.True(t, ok)
	require.Equal(t, "你好", zhNs["greeting"])

	filtered := loadPluginMenuI18n(context.Background(), plugin, []string{"en"})
	require.NotNil(t, filtered)
	require.Len(t, filtered.Locales, 1)
	require.Contains(t, filtered.Locales, "en")
	require.NotContains(t, filtered.Locales, "zh-CN")
}
