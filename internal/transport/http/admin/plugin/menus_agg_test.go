package plugin

import (
	"testing"

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
				Route: "plugins/base/dashboard",
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
