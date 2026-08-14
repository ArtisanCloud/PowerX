package iam

import (
	"context"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPermissionServiceListPluginCatalogGroupsPluginPermissions(t *testing.T) {
	ctx := context.Background()
	db := newPermissionServiceTestDB(t)
	require.NoError(t, db.Create(&[]dbm.Permission{
		{
			Module:   "menu.production",
			Resource: "sample_tracks",
			Action:   "view",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "plugin:demo.plugin",
			Meta: datatypes.JSON([]byte(`{
				"type":"menu",
				"module":"production",
				"plugin_id":"demo.plugin",
				"permission":"menu.production.sample_tracks:view",
				"menu_path":["production","sample_tracks"],
				"page_permission_codes":["production.sample_track:read"],
				"title_i18n":{"zh-CN":"小样跟踪单","en":"Sample tracking"},
				"description_i18n":{"zh-CN":"允许查看小样跟踪单菜单","en":"Allows viewing sample tracking menu"},
				"risk_level":"low",
				"data_scope":"tenant"
			}`)),
		},
		{
			Module:   "production",
			Resource: "sample_track",
			Action:   "read",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "plugin:demo.plugin",
			Meta: datatypes.JSON([]byte(`{
				"type":"page",
				"module":"production",
				"resource":"sample_track",
				"action":"read",
				"plugin_id":"demo.plugin",
				"permission":"production.sample_track:read",
				"title_i18n":{"zh-CN":"小样读取","en":"Read samples"},
				"description_i18n":{"zh-CN":"允许读取小样单","en":"Allows reading samples"},
				"risk_level":"low",
				"data_scope":"tenant",
				"protocol_bindings":[{"channel":"rest","method":"GET","path":"/sample-tracks","actor_context":"admin_user","resource_scope":"tenant"}]
			}`)),
		},
		{
			Module:   "production",
			Resource: "sample_track",
			Action:   "factory_schedule",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "plugin:demo.plugin",
			Meta: datatypes.JSON([]byte(`{
				"type":"action",
				"module":"production",
				"resource":"sample_track",
				"action":"factory_schedule",
				"plugin_id":"demo.plugin",
				"permission":"production.sample_track:factory_schedule",
				"title_i18n":{"zh-CN":"小样打样排产","en":"Sample schedule"},
				"description_i18n":{"zh-CN":"允许提交小样打样排产节点","en":"Allows submitting sample schedule node"},
				"risk_level":"medium",
				"data_scope":"tenant",
				"default_role_grants":["role_user"]
			}`)),
		},
		{
			Module:   "production",
			Resource: "sample_track_api",
			Action:   "sample_schedule",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "plugin:demo.plugin",
			Meta: datatypes.JSON([]byte(`{
				"type":"api",
				"module":"production",
				"resource":"sample_track_api",
				"action":"sample_schedule",
				"plugin_id":"demo.plugin",
				"permission":"production.sample_track_api:sample_schedule",
				"title_i18n":{"zh-CN":"小样打样排产接口","en":"Sample schedule API"},
				"description_i18n":{"zh-CN":"允许调用小样打样排产接口","en":"Allows calling sample schedule API"},
				"risk_level":"medium",
				"data_scope":"tenant",
				"business_permission_code":"production.sample_track:factory_schedule",
				"protocol_bindings":[{"channel":"rest","method":"POST","path":"/sample-tracks/*/nodes/sample-schedule","actor_context":"admin_user","resource_scope":"tenant"}]
			}`)),
		},
		{
			Module:   "runtime",
			Resource: "contract",
			Action:   "tenant_context",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "plugin:demo.plugin",
			Meta: datatypes.JSON([]byte(`{
				"type":"api",
				"module":"runtime",
				"resource":"contract",
				"action":"tenant_context",
				"plugin_id":"demo.plugin",
				"permission":"runtime.contract:tenant_context",
				"title_i18n":{"zh-CN":"租户上下文运行时合同","en":"Tenant context runtime contract"},
				"description_i18n":{"zh-CN":"PowerX 与插件之间的 ws-bus 租户上下文合同","en":"WS-bus tenant context contract between PowerX and the plugin"},
				"risk_level":"low",
				"data_scope":"tenant",
				"protocol_bindings":[{"channel":"rest","method":"POST","path":"/admin/runtime/ws-bus/grant","actor_context":"admin_user","resource_scope":"tenant"}]
			}`)),
		},
		{
			Module:   "system",
			Resource: "tenant",
			Action:   "manage",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "core",
		},
	}).Error)

	catalog, err := NewPermissionService(db).ListPluginCatalog(ctx, PluginPermissionCatalogFilter{
		PluginID: "demo.plugin",
		Module:   "production",
	})
	require.NoError(t, err)
	require.Len(t, catalog.Plugins, 1)
	require.Equal(t, "demo.plugin", catalog.Plugins[0].PluginID)
	require.Len(t, catalog.Plugins[0].MenuTree, 1)
	require.Equal(t, "production", catalog.Plugins[0].MenuTree[0].Key)
	require.Len(t, catalog.Plugins[0].MenuTree[0].Children, 1)
	require.Equal(t, "sample_tracks", catalog.Plugins[0].MenuTree[0].Children[0].Key)
	require.Equal(t, []string{"production.sample_track:read"}, catalog.Plugins[0].MenuTree[0].Children[0].PagePermissionCodes)
	require.Len(t, catalog.Plugins[0].BusinessModules, 1)
	require.Equal(t, "production", catalog.Plugins[0].BusinessModules[0].Module)
	require.Len(t, catalog.Plugins[0].BusinessModules[0].Resources, 1)
	require.Len(t, catalog.Plugins[0].BusinessModules[0].Resources[0].Pages, 1)
	require.Len(t, catalog.Plugins[0].BusinessModules[0].Resources[0].Actions, 1)
	require.Equal(t, "production.sample_track:factory_schedule", catalog.Plugins[0].BusinessModules[0].Resources[0].Actions[0].PermissionCode)
	require.Equal(t, map[string]string{"zh-CN": "小样打样排产", "en": "Sample schedule"}, catalog.Plugins[0].BusinessModules[0].Resources[0].Actions[0].TitleI18n)
	require.Equal(t, "registered", catalog.Plugins[0].BusinessModules[0].Resources[0].Actions[0].RegistrationStatus)
	require.Len(t, catalog.Plugins[0].APIBindings, 1)
	require.Equal(t, "production.sample_track:factory_schedule", catalog.Plugins[0].APIBindings[0].BusinessPermissionCode)
	require.Empty(t, catalog.Plugins[0].RuntimeContracts)

	fullCatalog, err := NewPermissionService(db).ListPluginCatalog(ctx, PluginPermissionCatalogFilter{
		PluginID: "demo.plugin",
	})
	require.NoError(t, err)
	require.Len(t, fullCatalog.Plugins, 1)
	require.Len(t, fullCatalog.Plugins[0].RuntimeContracts, 1)
	require.Equal(t, "runtime.contract:tenant_context", fullCatalog.Plugins[0].RuntimeContracts[0].PermissionCode)
	require.Len(t, fullCatalog.Plugins[0].BusinessModules, 1)
	require.Len(t, fullCatalog.Plugins[0].APIBindings, 1)
}

func TestPermissionServiceCleanupInvalidPluginPermissionsDeletesOnlyInvalidPluginRows(t *testing.T) {
	ctx := context.Background()
	db := newPermissionServiceTestDB(t)
	rows := []dbm.Permission{
		{
			Module:   "menu",
			Resource: "plugin.demo.invalid",
			Action:   "read",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "plugin:demo.plugin",
			Meta: datatypes.JSON([]byte(`{
				"type":"menu",
				"module":"menu",
				"plugin_id":"demo.plugin",
				"permission":"menu.plugin.demo.invalid:read"
			}`)),
		},
		{
			Module:   "production",
			Resource: "sample_track",
			Action:   "read",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "plugin:demo.plugin",
			Meta: datatypes.JSON([]byte(`{
				"type":"action",
				"module":"production",
				"plugin_id":"demo.plugin",
				"permission":"production.sample_track:read",
				"title_i18n":{"zh-CN":"小样读取"},
				"description_i18n":{"zh-CN":"允许读取小样单"},
				"risk_level":"low",
				"data_scope":"tenant",
				"resource":"sample_track",
				"action":"read"
			}`)),
		},
		{
			Module:   "menu",
			Resource: "plugin.other.invalid",
			Action:   "read",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "plugin:other.plugin",
			Meta: datatypes.JSON([]byte(`{
				"type":"menu",
				"module":"menu",
				"plugin_id":"other.plugin",
				"permission":"menu.plugin.other.invalid:read"
			}`)),
		},
	}
	require.NoError(t, db.Create(&rows).Error)
	require.NoError(t, db.Create(&dbm.RolePermission{RoleID: 10, PermissionID: rows[0].ID}).Error)
	require.NoError(t, db.Create(&dbm.RolePermission{RoleID: 10, PermissionID: rows[1].ID}).Error)

	result, err := NewPermissionService(db).CleanupInvalidPluginPermissions(ctx, "demo.plugin")
	require.NoError(t, err)
	require.Equal(t, "demo.plugin", result.PluginID)
	require.Equal(t, []uint64{rows[0].ID}, result.DeletedPermissionIDs)
	require.EqualValues(t, 1, result.DeletedPermissions)
	require.EqualValues(t, 1, result.DeletedBindings)

	var remaining []dbm.Permission
	require.NoError(t, db.Order("id ASC").Find(&remaining).Error)
	require.Len(t, remaining, 2)
	require.Equal(t, rows[1].ID, remaining[0].ID)
	require.Equal(t, rows[2].ID, remaining[1].ID)

	var bindingCount int64
	require.NoError(t, db.Model(&dbm.RolePermission{}).Count(&bindingCount).Error)
	require.EqualValues(t, 1, bindingCount)
}

func newPermissionServiceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})
	require.NoError(t, db.AutoMigrate(&dbm.Permission{}, &dbm.RolePermission{}))
	return db
}
