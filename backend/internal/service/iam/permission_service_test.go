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
			Module:   "production",
			Resource: "sample_track",
			Action:   "factory_schedule",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "plugin:demo.plugin",
			Meta: datatypes.JSON([]byte(`{
				"type":"action",
				"module":"production",
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
				"plugin_id":"demo.plugin",
				"permission":"production.sample_track_api:sample_schedule",
				"title_i18n":{"zh-CN":"小样打样排产接口","en":"Sample schedule API"},
				"description_i18n":{"zh-CN":"允许调用小样打样排产接口","en":"Allows calling sample schedule API"},
				"risk_level":"medium",
				"business_permission_code":"production.sample_track:factory_schedule"
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
	require.Len(t, catalog.Plugins[0].Modules, 1)
	require.Equal(t, "production", catalog.Plugins[0].Modules[0].Module)
	require.Len(t, catalog.Plugins[0].Modules[0].Types, 2)
	require.Equal(t, "action", catalog.Plugins[0].Modules[0].Types[0].Type)
	require.Equal(t, "production.sample_track:factory_schedule", catalog.Plugins[0].Modules[0].Types[0].Permissions[0].PermissionCode)
	require.Equal(t, map[string]string{"zh-CN": "小样打样排产", "en": "Sample schedule"}, catalog.Plugins[0].Modules[0].Types[0].Permissions[0].TitleI18n)
	require.Equal(t, "registered", catalog.Plugins[0].Modules[0].Types[0].Permissions[0].RegistrationStatus)
	require.Equal(t, "api", catalog.Plugins[0].Modules[0].Types[1].Type)
	require.Equal(t, "production.sample_track:factory_schedule", catalog.Plugins[0].Modules[0].Types[1].Permissions[0].BusinessPermissionCode)
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
	require.NoError(t, db.AutoMigrate(&dbm.Permission{}))
	return db
}
