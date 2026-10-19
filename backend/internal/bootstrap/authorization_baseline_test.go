package bootstrap

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStartupInitializesTrackingButReadOnlyRecoveryDoesNot(t *testing.T) {
	previous := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() { model.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&im.AuthorizationTracking{}, &im.AuthorizationStream{}, &im.AuthorizationSegment{}, &im.User{}, &im.Member{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}, &tm.Tenant{}, &sm.PluginInstanceConfig{}))
	tenant := tm.Tenant{Key: uuid.NewString(), Status: tm.TenantStatusActive}
	require.NoError(t, db.Create(&tenant).Error)
	const plugin = "com.example.startup"
	require.NoError(t, db.Create(&im.Permission{Module: "work", Resource: "item", Action: "read", Source: "plugin:" + plugin, Meta: datatypes.JSON(`{"authorization_baseline":true}`)}).Error)
	require.NoError(t, db.Create(&sm.PluginInstanceConfig{TenantUUID: tenant.UUID.String(), PluginID: plugin, Key: "auth.credentials", Enabled: true}).Error)
	require.NoError(t, authorizationbaseline.Install(db))
	cfg := &config.Config{}
	for _, mode := range []string{"verify", "hold", "coordinator"} {
		cfg.Recovery.Mode = mode
		maintenance := ""
		if mode == "coordinator" {
			maintenance = "1"
		}
		t.Setenv("POWERX_COORDINATOR_MAINTENANCE", maintenance)
		require.NoError(t, initializeAuthorizationBaselines(context.Background(), cfg, db))
		var count int64
		require.NoError(t, db.Model(&im.AuthorizationTracking{}).Count(&count).Error)
		require.Zero(t, count)
	}
	cfg.Recovery.Mode = ""
	t.Setenv("POWERX_COORDINATOR_MAINTENANCE", "")
	require.NoError(t, initializeAuthorizationBaselines(context.Background(), cfg, db))
	var track im.AuthorizationTracking
	require.NoError(t, db.Where("tenant_uuid=?", tenant.UUID.String()).Take(&track).Error)
	require.Equal(t, "ready", track.Status)
	require.Equal(t, authorizationbaseline.AutomaticInitializer, track.InitializedBy)
	require.NoError(t, initializeAuthorizationBaselines(context.Background(), cfg, db))
	var repeated im.AuthorizationTracking
	require.NoError(t, db.Where("tenant_uuid=?", tenant.UUID.String()).Take(&repeated).Error)
	require.Equal(t, track, repeated)
	require.NoError(t, db.Migrator().DropTable(&im.AuthorizationSegment{}))
	require.ErrorIs(t, initializeAuthorizationBaselines(context.Background(), cfg, db), ErrBootstrapDependencyUnavailable)
}
