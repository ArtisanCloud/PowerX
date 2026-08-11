package bootstrap

import (
	"encoding/json"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestBuildPluginPermissionRowsIncludesFrontendAdminMenus(t *testing.T) {
	manifest := plugin_mgr.Manifest{
		ID:      "com.example.app",
		Version: "1.2.3",
		Name:    "Example App",
		Frontend: plugin_mgr.FrontendSpec{Admin: plugin_mgr.FrontendAdminSpec{Menus: []plugin_mgr.MenuItem{
			{
				ID:    "templates",
				Title: "模板",
				Children: []plugin_mgr.MenuItem{
					{ID: "templates.detail", Title: "模板详情"},
				},
			},
		}}},
	}

	rows := buildPluginPermissionRows(manifest)

	require.Len(t, rows, 2)
	require.Equal(t, "menu", rows[0].Module)
	require.Equal(t, "plugin.com.example.app.templates", rows[0].Resource)
	require.Equal(t, "read", rows[0].Action)
	require.Equal(t, "plugin:com.example.app", rows[0].Source)
	require.False(t, rows[0].AllowAPIKey)

	var meta map[string]any
	require.NoError(t, json.Unmarshal(rows[0].Meta, &meta))
	require.Equal(t, "menu", meta["type"])
	require.Equal(t, "com.example.app", meta["plugin_id"])
	require.Equal(t, "Example App", meta["plugin_name"])
	require.Equal(t, "templates", meta["menu_id"])
	require.Equal(t, "模板", meta["label"])

	require.Equal(t, "plugin.com.example.app.templates.detail", rows[1].Resource)
}

func TestBuildPluginPermissionRowsKeepsManifestPermissionsAndMenuPermissions(t *testing.T) {
	manifest := plugin_mgr.Manifest{
		ID:      "com.example.app",
		Version: "1.2.3",
		Permissions: []plugin_mgr.PermissionSpec{
			{
				Resource: "template",
				Actions:  []string{"read", "write"},
				Module:   "template",
				Type:     "action",
			},
		},
		Frontend: plugin_mgr.FrontendSpec{Admin: plugin_mgr.FrontendAdminSpec{Menus: []plugin_mgr.MenuItem{
			{ID: "templates", Title: "模板"},
		}}},
	}

	rows := buildPluginPermissionRows(manifest)

	require.Len(t, rows, 3)
	require.Equal(t, "com.example.app", rows[0].Module)
	require.Equal(t, "template", rows[0].Resource)
	require.Equal(t, "read", rows[0].Action)
	require.Equal(t, "menu", rows[2].Module)
	require.Equal(t, "plugin.com.example.app.templates", rows[2].Resource)
	require.Equal(t, "read", rows[2].Action)
}

func TestBuildPluginPermissionRowsSupportsGranularPermissionDeclarations(t *testing.T) {
	manifest := plugin_mgr.Manifest{
		ID:      "com.example.plugins.workflow",
		Version: "1.0.0",
		Permissions: []plugin_mgr.PermissionSpec{
			{
				Type:           "page",
				PermissionCode: "workspace.case_file:read",
				Module:         "workspace",
				TitleI18n: map[string]string{
					"zh-CN": "示例记录读取",
					"en":    "Read example records",
				},
				DescriptionI18n: map[string]string{
					"zh-CN": "允许查看示例记录列表。",
					"en":    "Allows reading example records.",
				},
				RiskLevel: "low",
				DataScope: "tenant",
				ProtocolBindings: []plugin_mgr.PermissionProtocolBinding{
					{
						Channel:       "rest",
						Method:        "GET",
						Path:          "/admin/example/records",
						ActorContext:  "admin_user",
						ResourceScope: "tenant",
					},
				},
			},
			{
				Type:                   "api",
				PermissionCode:         "workspace.case_file_api:approve",
				BusinessPermissionCode: "workspace.case_file:approve",
				Module:                 "workspace",
				RiskLevel:              "medium",
				DataScope:              "tenant",
				ProtocolBindings: []plugin_mgr.PermissionProtocolBinding{
					{
						Channel:       "rest",
						Method:        "POST",
						Path:          "/admin/example/records/{uuid}/approve",
						ActorContext:  "admin_user",
						ResourceScope: "tenant",
					},
				},
			},
		},
	}

	rows := buildPluginPermissionRows(manifest)

	require.Len(t, rows, 2)
	require.Equal(t, "workspace", rows[0].Module)
	require.Equal(t, "case_file", rows[0].Resource)
	require.Equal(t, "read", rows[0].Action)
	require.Equal(t, "plugin:com.example.plugins.workflow", rows[0].Source)

	var pageMeta map[string]any
	require.NoError(t, json.Unmarshal(rows[0].Meta, &pageMeta))
	require.Equal(t, "page", pageMeta["type"])
	require.Equal(t, "workspace.case_file:read", pageMeta["permission"])
	require.Equal(t, "com.example.plugins.workflow", pageMeta["plugin_id"])
	require.Equal(t, "low", pageMeta["risk_level"])
	require.NotEmpty(t, pageMeta["protocol_bindings"])

	var apiMeta map[string]any
	require.NoError(t, json.Unmarshal(rows[1].Meta, &apiMeta))
	require.Equal(t, "api", apiMeta["type"])
	require.Equal(t, "workspace.case_file_api:approve", apiMeta["permission"])
	require.Equal(t, "workspace.case_file:approve", apiMeta["business_permission_code"])
	require.NotEmpty(t, apiMeta["protocol_bindings"])
}

func TestBuildPluginPermissionRowsAllowsLongPluginMenuResources(t *testing.T) {
	manifest := plugin_mgr.Manifest{
		ID:      "com.powerx.plugins.base",
		Version: "0.1.3",
		Name:    "PowerX Base",
		Frontend: plugin_mgr.FrontendSpec{Admin: plugin_mgr.FrontendAdminSpec{Menus: []plugin_mgr.MenuItem{
			{
				ID:    "plugins.com.powerx.plugins.base.templates.framework_lab",
				Title: "Framework Lab",
			},
		}}},
	}

	rows := buildPluginPermissionRows(manifest)

	require.Len(t, rows, 1)
	require.Greater(t, len(rows[0].Resource), 64)
	require.Equal(t, "plugin.com.powerx.plugins.base.plugins.com.powerx.plugins.base.templates.framework_lab", rows[0].Resource)
	require.Equal(t, "menu", rows[0].Module)
	require.Equal(t, "read", rows[0].Action)
}
