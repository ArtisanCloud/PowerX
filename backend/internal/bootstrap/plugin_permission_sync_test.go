package bootstrap

import (
	"encoding/json"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestBuildPluginPermissionRowsIgnoresFrontendAdminMenusWithoutPermissionDeclarations(t *testing.T) {
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

	require.Empty(t, rows)
}

func TestBuildPluginPermissionRowsKeepsOnlyManifestPermissions(t *testing.T) {
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

	require.Len(t, rows, 2)
	require.Equal(t, "com.example.app", rows[0].Module)
	require.Equal(t, "template", rows[0].Resource)
	require.Equal(t, "read", rows[0].Action)
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
				Resource:       "case_file",
				Action:         "read",
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
				Resource:               "case_file_api",
				Action:                 "approve",
				RiskLevel:              "medium",
				DataScope:              "tenant",
				ProtocolBindings: []plugin_mgr.PermissionProtocolBinding{
					{
						Channel:       "rest",
						Method:        "POST",
						Path:          "/admin/example/records/*/approve",
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

func TestBuildPluginPermissionRowsDoesNotAutoGeneratePluginMenuPermissions(t *testing.T) {
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

	require.Empty(t, rows)
}

func TestBuildPluginPermissionRowsFlagsMenuPermissionMissingDeclaration(t *testing.T) {
	manifest := plugin_mgr.Manifest{
		ID:      "com.example.app",
		Version: "1.2.3",
		Frontend: plugin_mgr.FrontendSpec{Admin: plugin_mgr.FrontendAdminSpec{Menus: []plugin_mgr.MenuItem{
			{
				ID: "business",
				Children: []plugin_mgr.MenuItem{{
					ID:               "records",
					RequiredPolicies: []string{"menu.example.records:view"},
				}},
			},
		}}},
	}

	rows := buildPluginPermissionRows(manifest)

	require.Len(t, rows, 1)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(rows[0].Meta, &meta))
	require.Equal(t, "menu.example.records:view", meta["permission"])
	require.Equal(t, []any{"business", "records"}, meta["menu_path"])
	require.Contains(t, meta["registration_errors"], "menu_permission_declaration_missing")
}

func TestBuildPluginPermissionRowsFlagsOrphanMenuPermission(t *testing.T) {
	manifest := plugin_mgr.Manifest{
		ID:      "com.example.app",
		Version: "1.2.3",
		Permissions: []plugin_mgr.PermissionSpec{validMenuPermissionSpec(
			"menu.example.records:view",
			[]string{"business", "records"},
		)},
	}

	rows := buildPluginPermissionRows(manifest)

	require.Len(t, rows, 1)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(rows[0].Meta, &meta))
	require.Contains(t, meta["registration_errors"], "menu_permission_orphan")
}

func TestBuildPluginPermissionRowsFlagsMenuPathMismatch(t *testing.T) {
	manifest := plugin_mgr.Manifest{
		ID:      "com.example.app",
		Version: "1.2.3",
		Frontend: plugin_mgr.FrontendSpec{Admin: plugin_mgr.FrontendAdminSpec{Menus: []plugin_mgr.MenuItem{
			{
				ID: "business",
				Children: []plugin_mgr.MenuItem{{
					ID:               "records",
					RequiredPolicies: []string{"menu.example.records:view"},
				}},
			},
		}}},
		Permissions: []plugin_mgr.PermissionSpec{validMenuPermissionSpec(
			"menu.example.records:view",
			[]string{"records"},
		)},
	}

	rows := buildPluginPermissionRows(manifest)

	require.Len(t, rows, 1)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(rows[0].Meta, &meta))
	require.Contains(t, meta["registration_errors"], "menu_path_mismatch")
	require.Contains(t, meta["registration_errors"], "menu_path_expected:business/records")
	require.Contains(t, meta["registration_errors"], "menu_path_actual:records")
}

func TestBuildPluginPermissionRowsFlagsTechnicalMenuPath(t *testing.T) {
	manifest := plugin_mgr.Manifest{
		ID:      "com.example.app",
		Version: "1.2.3",
		Frontend: plugin_mgr.FrontendSpec{Admin: plugin_mgr.FrontendAdminSpec{Menus: []plugin_mgr.MenuItem{
			{
				ID:               "_p/com.example.app/admin/records",
				RequiredPolicies: []string{"menu.example.records:view"},
			},
		}}},
		Permissions: []plugin_mgr.PermissionSpec{validMenuPermissionSpec(
			"menu.example.records:view",
			[]string{"_p/com.example.app/admin/records"},
		)},
	}

	rows := buildPluginPermissionRows(manifest)

	require.Len(t, rows, 1)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(rows[0].Meta, &meta))
	require.Contains(t, meta["registration_errors"], "menu_path_invalid_technical_segment")
}

func validMenuPermissionSpec(code string, menuPath []string) plugin_mgr.PermissionSpec {
	return plugin_mgr.PermissionSpec{
		Type:           "menu",
		PermissionCode: code,
		Module:         "example",
		MenuPath:       menuPath,
		TitleI18n: map[string]string{
			"zh-CN": "示例菜单",
		},
		DescriptionI18n: map[string]string{
			"zh-CN": "允许查看示例菜单。",
		},
		RiskLevel: "low",
		DataScope: "tenant",
	}
}
