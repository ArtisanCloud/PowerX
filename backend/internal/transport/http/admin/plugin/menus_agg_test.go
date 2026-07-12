package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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
		ID:               "base",
		Title:            "基础插件",
		Route:            "plugins/base",
		RequiredPolicies: []string{"example:template:read"},
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
	item := convertPluginMenuItem("com.example.base", "0.8.6", "/_p/com.example.base/admin/", "", menu, locales, bundle)

	require.Equal(t, plugin_mgr.MenuKey("plugin:com.example.base:base"), item.Key)
	require.Equal(t, "Base Plugin", item.Title)
	require.Equal(t, "0.8.6", item.PluginVersion)
	require.ElementsMatch(t, []string{
		"example:template:read",
		"menu:plugin.com.example.base.base:read",
	}, item.Permissions)
	require.Len(t, item.Children, 1)
	child := item.Children[0]
	require.Equal(t, item.Key, child.ParentID)
	require.Equal(t, "Dashboard", child.Title)
	require.ElementsMatch(t, []string{
		"menu:plugin.com.example.base.base.dashboard:read",
	}, child.Permissions)
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

func TestBuildPluginMenusPublicKeepsRootTenantContextMenus(t *testing.T) {
	origCfg := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = origCfg })
	config.GlobalConfig = &config.Config{
		Plugin: config.PluginAggregateConfig{
			PluginConfig: config.PluginConfig{Enabled: true},
		},
	}

	origChecker := tenantPluginChecker.fn
	SetTenantPluginEnabledChecker(func(ctx context.Context, tenantUUID, pluginID string) (bool, error) {
		return tenantUUID == "00000000-0000-0000-0000-000000000001" && pluginID == "com.powerx.plugins.base", nil
	})
	t.Cleanup(func() { SetTenantPluginEnabledChecker(origChecker) })

	pmimpl.ResetGlobalForTest(&fakeMenuManager{plugins: []plugin_mgr.Plugin{
		{
			ID:      "com.powerx.plugins.base",
			Version: "0.1.4",
			State:   plugin_mgr.StateEnabled,
			Frontend: plugin_mgr.FrontendSpec{Admin: plugin_mgr.FrontendAdminSpec{Menus: []plugin_mgr.MenuItem{
				{ID: "intro", Title: "彩蛋", Route: "/", Slot: plugin_mgr.SlotPlugins},
			}}},
		},
	}})
	t.Cleanup(func() { pmimpl.ResetGlobalForTest(nil) })

	ctx := context.Background()
	ctx = reqctx.WithIsRoot(ctx, true)
	ctx = reqctx.WithTenantUUID(ctx, "00000000-0000-0000-0000-000000000001")

	out := BuildPluginMenusPublic(ctx, "/_p", []string{"zh-CN"})
	require.Len(t, out.Items, 1)
	require.Equal(t, plugin_mgr.MenuKey("plugin:com.powerx.plugins.base:intro"), out.Items[0].Key)
	require.Equal(t, "/_p/com.powerx.plugins.base/admin/", out.Items[0].URL)
}

type fakeMenuManager struct {
	plugins []plugin_mgr.Plugin
}

func (m *fakeMenuManager) Bootstrap(ctx context.Context) error { return nil }
func (m *fakeMenuManager) Shutdown(ctx context.Context) error  { return nil }
func (m *fakeMenuManager) SwitchVersion(ctx context.Context, id, version string, enable bool) (plugin_mgr.Plugin, error) {
	return plugin_mgr.Plugin{}, nil
}
func (m *fakeMenuManager) InstallFromFile(ctx context.Context, path string, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	return plugin_mgr.Plugin{}, nil
}
func (m *fakeMenuManager) InstallFromURL(ctx context.Context, url, sha256, signature string, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	return plugin_mgr.Plugin{}, nil
}
func (m *fakeMenuManager) Upgrade(ctx context.Context, id, version string, src plugin_mgr.InstallSource, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	return plugin_mgr.Plugin{}, nil
}
func (m *fakeMenuManager) Enable(ctx context.Context, id string) error  { return nil }
func (m *fakeMenuManager) Disable(ctx context.Context, id string) error { return nil }
func (m *fakeMenuManager) Uninstall(ctx context.Context, id string, versionOptional ...string) error {
	return nil
}
func (m *fakeMenuManager) UninstallAndPurge(ctx context.Context, id string, versionOptional ...string) error {
	return nil
}
func (m *fakeMenuManager) List(ctx context.Context) ([]plugin_mgr.Plugin, error) {
	return append([]plugin_mgr.Plugin(nil), m.plugins...), nil
}
func (m *fakeMenuManager) Get(ctx context.Context, id string) (plugin_mgr.Plugin, error) {
	for _, p := range m.plugins {
		if p.ID == id {
			return p, nil
		}
	}
	return plugin_mgr.Plugin{}, nil
}
