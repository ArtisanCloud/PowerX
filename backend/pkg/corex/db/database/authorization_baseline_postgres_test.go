package database

import (
	"os"
	"strings"
	"testing"

	coredb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	baseline "github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Uses a disposable database, not only a schema: the full migration also touches
// fixed public tables. Never point this at a development or production database.
func TestPostgresMigrationAutomaticallyInitializesAllTenants(t *testing.T) {
	dsn := os.Getenv("POWERX_BASELINE_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("POWERX_BASELINE_MIGRATION_TEST_DSN is not set")
	}
	old := model.PowerXSchema
	model.PowerXSchema = "public"
	t.Cleanup(func() { model.PowerXSchema = old })
	legacy, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	sqlDB, err := legacy.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var name string
	require.NoError(t, legacy.Raw("SELECT current_database()").Scan(&name).Error)
	require.True(t, strings.HasPrefix(name, "powerx_baseline_migration_test_"), "a dedicated disposable database is required")
	db, err := Connect(coredb.DatabaseConfig{Driver: "postgres", DSN: dsn, LogLevel: "silent", MaxOpenConns: 8})
	require.NoError(t, err)
	managedSQL, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, managedSQL.Close()) })
	// A fresh installation must migrate successfully before any tenants exist.
	require.NoError(t, MigrateCoreModels(db))
	plugin := "com.example.migration." + uuid.NewString()
	permission := im.Permission{Module: uuid.NewString(), Resource: "order", Action: "read", Source: "plugin:" + plugin, Meta: datatypes.JSON(`{"authorization_baseline":true,"data_scope":"TENANT"}`)}
	require.NoError(t, legacy.Create(&permission).Error)
	var tenants []string
	for range 2 {
		tenant := tm.Tenant{Key: uuid.NewString(), Status: tm.TenantStatusActive}
		require.NoError(t, legacy.Create(&tenant).Error)
		tenants = append(tenants, tenant.UUID.String())
		require.NoError(t, legacy.Create(&sm.PluginInstanceConfig{TenantUUID: tenant.UUID.String(), PluginID: plugin, Key: "auth.credentials", Enabled: true}).Error)
		user := im.User{Email: uuid.NewString() + "@example.invalid", Status: 1}
		require.NoError(t, legacy.Create(&user).Error)
		member := im.Member{TenantUUID: tenant.UUID.String(), UserID: user.ID, UserUUID: user.UUID.String(), Username: "member", DisplayName: uuid.NewString(), Status: 1}
		require.NoError(t, legacy.Create(&member).Error)
		role := im.Role{TenantUUID: tenant.UUID.String(), Scope: "tenant", Code: "reader"}
		require.NoError(t, legacy.Create(&role).Error)
		require.NoError(t, legacy.Create(&im.RolePermission{RoleID: role.ID, RoleUUID: role.UUID, PermissionID: permission.ID, PermissionUUID: permission.PermissionUUID}).Error)
		require.NoError(t, legacy.Create(&im.RoleBinding{TenantUUID: tenant.UUID.String(), RoleID: role.ID, RoleUUID: role.UUID.String(), SubjectType: im.SubMember, SubjectID: member.ID, SubjectUUID: member.UUID.String(), DataScope: im.ScopeTenant}).Error)
	}
	var count int64
	require.NoError(t, legacy.Model(&im.AuthorizationTracking{}).Where("plugin_id=?", plugin).Count(&count).Error)
	require.Zero(t, count, "old installations have no automatic tracking")
	require.NoError(t, MigrateCoreModels(db))
	var tracks []im.AuthorizationTracking
	require.NoError(t, db.Where("plugin_id=?", plugin).Order("tenant_uuid").Find(&tracks).Error)
	require.Len(t, tracks, 2)
	for _, track := range tracks {
		require.Contains(t, tenants, track.TenantUUID)
		require.Equal(t, "ready", track.Status)
		require.Equal(t, baseline.AutomaticInitializer, track.InitializedBy)
		require.NotNil(t, track.KnownSince)
	}
	var segments []im.AuthorizationSegment
	require.NoError(t, db.Where("tenant_uuid IN ?", tenants).Find(&segments).Error)
	require.Len(t, segments, 2)
	for _, segment := range segments {
		require.Equal(t, baseline.Allowed, segment.State)
		require.Equal(t, "initialized", segment.BaselineKind)
		require.Nil(t, segment.GrantedAt)
	}
	require.NoError(t, MigrateCoreModels(db))
	var repeated []im.AuthorizationTracking
	require.NoError(t, db.Where("plugin_id=?", plugin).Order("tenant_uuid").Find(&repeated).Error)
	require.Equal(t, tracks, repeated)
	var after []im.AuthorizationSegment
	require.NoError(t, db.Where("tenant_uuid IN ?", tenants).Find(&after).Error)
	require.Equal(t, segments, after)
	// A later tenant and plugin enable use the same factory, without another migrate/seed.
	tenant := tm.Tenant{Key: uuid.NewString(), Status: tm.TenantStatusActive}
	require.NoError(t, db.Create(&tenant).Error)
	require.NoError(t, db.Create(&sm.PluginInstanceConfig{TenantUUID: tenant.UUID.String(), PluginID: plugin, Key: "auth.credentials", Enabled: true}).Error)
	require.NoError(t, db.Model(&im.AuthorizationTracking{}).Where("plugin_id=?", plugin).Count(&count).Error)
	require.EqualValues(t, 3, count)
	require.NoError(t, db.Model(&im.RoleBinding{}).Where("tenant_uuid IN ?", tenants).Count(&count).Error)
	require.EqualValues(t, 2, count, "migration must not add role grants")
}
