package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	coredb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
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

func TestInstalledCoreUpgradeMigratesLegacyRootBaselineWithNewIdentity(t *testing.T) {
	dsn := os.Getenv("POWERX_UPGRADE_BASELINE_TEST_DSN")
	if dsn == "" {
		t.Skip("POWERX_UPGRADE_BASELINE_TEST_DSN requires a disposable database")
	}
	oldSchema := model.PowerXSchema
	model.PowerXSchema = "public"
	t.Cleanup(func() { model.PowerXSchema = oldSchema })
	legacy, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	legacySQL, err := legacy.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, legacySQL.Close()) })
	var databaseName string
	require.NoError(t, legacy.Raw("SELECT current_database()").Scan(&databaseName).Error)
	require.True(t, strings.HasPrefix(databaseName, "powerx_upgrade_test_iam_baseline_"), "never use a development or production database")
	db, err := database.Connect(coredb.DatabaseConfig{Driver: "postgres", DSN: dsn, LogLevel: "silent", MaxOpenConns: 8})
	require.NoError(t, err)
	managedSQL, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, managedSQL.Close()) })
	require.NoError(t, database.MigrateCoreModels(db))
	require.NoError(t, database.MigrateUpgradeJournal(db))

	// 旧程序写入已有租户和 root 流，绕开新版本的授权源事务钩子。
	plugin := "com.example.upgrade." + uuid.NewString()
	permission := im.Permission{Module: "upgrade_" + uuid.NewString(), Resource: "cost", Action: "read", Effect: "allow", Status: im.PermissionStatusActive, Source: "plugin:" + plugin, Meta: datatypes.JSON(`{"authorization_baseline":true,"data_scope":"TENANT"}`)}
	require.NoError(t, legacy.Create(&permission).Error)
	tenant := tm.Tenant{Key: uuid.NewString(), Status: tm.TenantStatusActive}
	require.NoError(t, legacy.Create(&tenant).Error)
	require.NoError(t, legacy.Create(&sm.PluginInstanceConfig{TenantUUID: tenant.UUID.String(), PluginID: plugin, Key: "auth.credentials", Enabled: true}).Error)
	user := im.User{Email: uuid.NewString() + "@example.invalid", IsRoot: true, Status: im.UserStatusActive}
	require.NoError(t, legacy.Create(&user).Error)
	member := im.Member{TenantUUID: tenant.UUID.String(), UserID: user.ID, UserUUID: user.UUID.String(), Username: "root", Status: 1}
	require.NoError(t, legacy.Create(&member).Error)
	past := baseline.Micro(time.Now().Add(-48 * time.Hour))
	track := im.AuthorizationTracking{TenantUUID: tenant.UUID.String(), PluginID: plugin, PermissionCode: permission.Module + ".cost:read", Status: "ready", KnownSince: &past}
	require.NoError(t, legacy.Create(&track).Error)
	require.NoError(t, legacy.First(&track, track.ID).Error)
	require.NoError(t, baseline.Reconcile(legacy, past))
	var oldStream im.AuthorizationStream
	require.NoError(t, legacy.Where("member_uuid=?", member.UUID).Take(&oldStream).Error)
	var oldSegment im.AuthorizationSegment
	require.NoError(t, legacy.Where("stream_uuid=? AND effective_until IS NULL", oldStream.UUID.String()).Take(&oldSegment).Error)
	require.Equal(t, baseline.RootBypass, oldSegment.State)

	// dc858393 中已提交的清单：旧成功履历必须保留，新内容不能被跳过。
	const oldChecksum = "140b925afeb7b8c9e80b9fb28933218b885dcdd146c2d6a612503b503edd7342"
	oldID := "20261011_core_" + oldChecksum
	require.NoError(t, db.Exec("INSERT INTO public.database_migration_records (migration_id, checksum, release_id, applied_at) VALUES (?, ?, ?, ?)", oldID, oldChecksum, "legacy-baseline", past).Error)
	steps, version, err := installedUpgradeSteps(&config.Config{})
	require.NoError(t, err)
	core := steps[0]
	require.True(t, strings.HasPrefix(core.ID, "20261011_core_"))
	require.Equal(t, "20261011_core_"+core.Checksum, core.ID)
	require.NotEqual(t, oldID, core.ID)
	runner := &database.UpgradeRunner{DB: db, SourceSHA: strings.Repeat("1", 40), ReleaseID: "baseline-regression", MigrationVersion: version, Steps: []database.UpgradeStep{core}}
	ctx := context.Background()
	plan, err := runner.Plan(ctx, "plan")
	require.NoError(t, err)
	require.Empty(t, plan.Blockers)
	require.Len(t, plan.Pending, 1)
	require.Equal(t, core.ID, plan.Pending[0].ID)
	before := baseline.Micro(time.Now())
	result, err := runner.Apply(ctx, plan.PlanHash, time.Second)
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Status, "%+v", result)
	require.Equal(t, []string{core.ID}, result.Applied)

	var current im.AuthorizationSegment
	require.NoError(t, db.Where("stream_uuid=? AND effective_until IS NULL", oldSegment.StreamUUID).Take(&current).Error)
	require.Equal(t, baseline.Allowed, current.State)
	require.Equal(t, "native", current.BaselineKind)
	require.NotNil(t, current.GrantedAt)
	require.False(t, current.GrantedAt.Before(before), "upgrade must not backdate new root grants")
	var historical im.AuthorizationSegment
	require.NoError(t, db.Where("uuid=?", oldSegment.UUID).Take(&historical).Error)
	require.Equal(t, oldSegment.State, historical.State)
	require.True(t, past.Equal(historical.EffectiveFrom))
	require.Equal(t, current.EffectiveFrom, *historical.EffectiveUntil)
	var afterTrack im.AuthorizationTracking
	require.NoError(t, db.First(&afterTrack, track.ID).Error)
	require.Equal(t, track, afterTrack, "known_since and initialization receipt must remain unchanged")
	var bindings int64
	require.NoError(t, db.Model(&im.RoleBinding{}).Where("tenant_uuid=?", tenant.UUID.String()).Count(&bindings).Error)
	require.Zero(t, bindings, "upgrade must not add business role grants")
	var oldRecordedChecksum string
	require.NoError(t, db.Raw("SELECT checksum FROM public.database_migration_records WHERE migration_id=?", oldID).Scan(&oldRecordedChecksum).Error)
	require.Equal(t, oldChecksum, oldRecordedChecksum)

	plan, err = runner.Plan(ctx, "plan")
	require.NoError(t, err)
	require.Empty(t, plan.Pending)
	result, err = runner.Apply(ctx, plan.PlanHash, time.Second)
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Status)
	require.Empty(t, result.Applied)
	require.Equal(t, []string{core.ID}, result.Skipped)
	var repeated im.AuthorizationSegment
	require.NoError(t, db.Where("uuid=?", current.UUID).Take(&repeated).Error)
	require.Equal(t, current, repeated)
	var coreRecords int64
	require.NoError(t, db.Table("public.database_migration_records").Where("migration_id IN ?", []string{oldID, core.ID}).Count(&coreRecords).Error)
	require.EqualValues(t, 2, coreRecords)
}
