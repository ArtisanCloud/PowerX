package bootstrap

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

func TestEffectivePermissionCodeFromIAMRowUsesBusinessPermissionForAPI(t *testing.T) {
	row := dbm.Permission{
		Module:   "workspace",
		Resource: "case_file_api",
		Action:   "approve",
		Meta: datatypes.JSON([]byte(`{
			"type":"api",
			"permission":"workspace.case_file_api:approve",
			"business_permission_code":"workspace.case_file:approve"
		}`)),
	}

	require.Equal(t, "workspace.case_file:approve", effectivePermissionCodeFromIAMRow(row))
}

func TestEffectivePermissionCodeFromIAMRowKeepsIndependentAPI(t *testing.T) {
	row := dbm.Permission{
		Module:   "workspace",
		Resource: "audit_api",
		Action:   "export",
		Meta: datatypes.JSON([]byte(`{
			"type":"api",
			"permission":"workspace.audit_api:export",
			"independent":true
		}`)),
	}

	require.Equal(t, "workspace.audit_api:export", effectivePermissionCodeFromIAMRow(row))
}

func TestEffectivePermissionCodeFromIAMRowRejectsAPIWithoutEffectivePermission(t *testing.T) {
	row := dbm.Permission{
		Module:   "workspace",
		Resource: "case_file_api",
		Action:   "read",
		Meta:     datatypes.JSON([]byte(`{"type":"api","permission":"workspace.case_file_api:read"}`)),
	}

	require.Empty(t, effectivePermissionCodeFromIAMRow(row))
}

func TestPluginIAMAuthorizerRoutePermissionUsesEffectivePermission(t *testing.T) {
	db := newPluginAuthzTestDB(t)
	require.NoError(t, db.Create(&dbm.Permission{
		Module:   "workspace",
		Resource: "case_file_api",
		Action:   "approve",
		Effect:   "allow",
		Status:   dbm.PermissionStatusActive,
		Source:   "plugin:demo.plugin",
		Meta: datatypes.JSON([]byte(`{
			"type":"api",
			"permission":"workspace.case_file_api:approve",
			"business_permission_code":"workspace.case_file:approve",
			"protocol_bindings":[
				{"channel":"rest","method":"POST","path":"/admin/example/records/*/approve","actor_context":"admin_user","resource_scope":"tenant"}
			]
		}`)),
	}).Error)

	permission, err := (pluginIAMAuthorizer{db: db}).RoutePermission(
		context.Background(),
		"demo.plugin",
		"POST",
		"/admin/example/records/42/approve",
	)

	require.NoError(t, err)
	require.NotNil(t, permission)
	require.Equal(t, "workspace", permission.Module)
	require.Equal(t, "case_file", permission.Resource)
	require.Equal(t, "approve", permission.Action)
}

func TestPluginIAMAuthorizerRoutePermissionRequiresPluginSource(t *testing.T) {
	db := newPluginAuthzTestDB(t)
	rows := []dbm.Permission{
		{
			Module:   "workspace",
			Resource: "core_case_file_api",
			Action:   "approve",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "core",
			Meta: datatypes.JSON([]byte(`{
				"type":"api",
				"permission":"workspace.case_file_api:approve",
				"business_permission_code":"workspace.case_file:approve",
				"protocol_bindings":[
					{"channel":"rest","method":"POST","path":"/admin/example/records/*/approve","actor_context":"admin_user","resource_scope":"tenant"}
				]
			}`)),
		},
		{
			Module:   "workspace",
			Resource: "other_case_file_api",
			Action:   "approve",
			Effect:   "allow",
			Status:   dbm.PermissionStatusActive,
			Source:   "plugin:other.plugin",
			Meta: datatypes.JSON([]byte(`{
				"type":"api",
				"permission":"workspace.case_file_api:approve",
				"business_permission_code":"workspace.case_file:approve",
				"protocol_bindings":[
					{"channel":"rest","method":"POST","path":"/admin/example/records/*/approve","actor_context":"admin_user","resource_scope":"tenant"}
				]
			}`)),
		},
	}
	require.NoError(t, db.Create(&rows).Error)

	permission, err := (pluginIAMAuthorizer{db: db}).RoutePermission(
		context.Background(),
		"demo.plugin",
		"POST",
		"/admin/example/records/42/approve",
	)

	require.NoError(t, err)
	require.Nil(t, permission)
}

func newPluginAuthzTestDB(t *testing.T) *gorm.DB {
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
