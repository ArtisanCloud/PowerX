package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestConvertPluginMenuItemHierarchy(t *testing.T) {
	bundle := &admdto.MenuI18nPackage{
		DefaultNamespace: "menus",
		Locales: admdto.MenuI18nLocales{
			"en": {
				"menus": map[string]any{
					"menu": map[string]any{
						"base": map[string]any{
							"title":     "Base Plugin",
							"dashboard": "Dashboard",
						},
					},
				},
			},
		},
	}

	menu := plugin_mgr.MenuItem{
		ID:    "base",
		Title: "基础插件",
		Route: "plugins/base",
		Children: []plugin_mgr.MenuItem{
			{
				ID:    "base.dashboard",
				Title: "仪表盘",
				Route: "dashboard",
				TitleI18n: &plugin_mgr.MenuLabel{
					Namespace: "menus",
					Key:       "menu.base.dashboard",
					Default:   "Dashboard",
				},
			},
		},
		TitleI18n: &plugin_mgr.MenuLabel{
			Namespace: "menus",
			Key:       "menu.base.title",
			Default:   "Base Plugin",
		},
	}

	locales := normalizeLocalePreference([]string{"en-US"})
	item := convertPluginMenuItem("com.example.base", "/_p/com.example.base/admin/", "", menu, locales, bundle)

	require.Equal(t, plugin_mgr.MenuKey("plugin:com.example.base:base"), item.Key)
	require.Equal(t, "Base Plugin", item.Title)
	require.Len(t, item.Children, 1)
	child := item.Children[0]
	require.Equal(t, item.Key, child.ParentID)
	require.Equal(t, "Dashboard", child.Title)
}

func TestBuildPluginMenusPublicReturnsEmptyWhenPluginDisabled(t *testing.T) {
	orig := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = orig })

	config.GlobalConfig = &config.Config{
		Plugin: config.PluginAggregateConfig{
			PluginConfig: config.PluginConfig{
				Enabled: false,
			},
		},
	}

	out := BuildPluginMenusPublic(context.Background(), "/_p", []string{"zh-CN"})
	require.Empty(t, out.Items)
	require.Empty(t, out.I18n)
}

func TestBuildPluginMenusPublicReadsConfigFromEtcWhenGlobalNil(t *testing.T) {
	orig := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = orig })
	config.GlobalConfig = nil

	tmp := t.TempDir()
	etc := filepath.Join(tmp, "etc")
	require.NoError(t, os.MkdirAll(etc, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(etc, "config.yaml"),
		[]byte("plugin:\n  enabled: false\n"),
		0o644,
	))

	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(wd) })

	out := BuildPluginMenusPublic(context.Background(), "/_p", nil)
	require.Empty(t, out.Items)
	require.Empty(t, out.I18n)
}
