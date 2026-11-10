package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	am "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestInstalledCatalogUpgradeMigratesLegacyBindings(t *testing.T) {
	dsn := os.Getenv("POWERX_CATALOG_INSTALLED_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable PostgreSQL required")
	}
	old := model.PowerXSchema
	model.PowerXSchema = "public"
	t.Cleanup(func() { model.PowerXSchema = old })
	dir, err := filepath.Abs("../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv("PLATFORM_CAPABILITIES_DIR", dir)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	var name string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&name).Error)
	require.True(t, strings.HasPrefix(name, "powerx_upgrade_test_catalog"))
	require.NoError(t, database.MigrateCoreModels(db))
	require.NoError(t, database.MigrateUpgradeJournal(db))
	for _, spec := range []struct{ id, path string }{{"com.corex.ai.catalog.providers.read", "/api/v1/admin/agents/providers"}, {"com.corex.ai.catalog.models.read", "/api/v1/admin/agents/models"}} {
		require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: spec.id, PluginID: "corex.platform", PluginVersion: "legacy", Title: spec.id, Status: "published", CapabilitiesHash: strings.Repeat("a", 64), ProtocolHash: "legacy"}).Error)
		for _, disabled := range []bool{false, true} {
			tenant := tm.Tenant{Key: uuid.NewString()}
			require.NoError(t, db.Create(&tenant).Error)
			policy := cm.RoutingPolicy{Strategy: "weighted"}
			require.NoError(t, db.Create(&policy).Error)
			status := "published"
			if disabled {
				status = "disabled"
			}
			reg := cm.CapabilityRegistration{CapabilityID: spec.id, TenantUUID: tenant.UUID.String(), Version: 1, Status: status, ContractRef: "backend/internal/transport/http/admin/agent/api.go", RoutingPolicyID: policy.UUID}
			require.NoError(t, db.Create(&reg).Error)
			require.NoError(t, db.Create(&cm.AdapterEndpoint{RegistrationID: reg.ID, CapabilityID: spec.id, TenantUUID: reg.TenantUUID, AdapterID: spec.id + "-old", TransportType: "http", Endpoint: spec.path, ServiceRef: spec.path, IsActive: true, Weight: 100, TimeoutMS: 5000, Labels: datatypes.JSON(`{"method":"GET"}`)}).Error)
		}
	}
	const oldChecksum = "04d9c2c034c7e20e50bbd72d822d2e2036b92db27f7c0d8f812fa261987d10af"
	oldID := "20261011_catalog_" + oldChecksum
	require.NoError(t, db.Exec("INSERT INTO public.database_migration_records (migration_id,checksum,release_id,applied_at) VALUES (?,?,?,?)", oldID, oldChecksum, "previous-release", time.Now().Add(-24*time.Hour)).Error)
	steps, version, err := installedUpgradeSteps(&config.Config{})
	require.NoError(t, err)
	// 其他步骤用正式成功履历表示已应用，实际执行新 catalog callback 与原依赖链。
	for _, step := range steps[:len(steps)-1] {
		require.NoError(t, db.Exec("INSERT INTO public.database_migration_records (migration_id,checksum,release_id,applied_at) VALUES (?,?,?,?)", step.ID, step.Checksum, "fixture-prerequisites", time.Now()).Error)
	}
	catalogStep := steps[len(steps)-1]
	require.NotEqual(t, oldID, catalogStep.ID)
	runner := &database.UpgradeRunner{DB: db, SourceSHA: strings.Repeat("1", 40), ReleaseID: "catalog-controlled-validation", MigrationVersion: version, Steps: steps}
	ctx := context.Background()
	plan, err := runner.Plan(ctx, "plan")
	require.NoError(t, err)
	require.Empty(t, plan.Blockers)
	require.Len(t, plan.Pending, 1)
	result, err := runner.Apply(ctx, plan.PlanHash, time.Second)
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Status)
	var upgraded []cm.CapabilityRegistration
	require.NoError(t, db.Where("capability_id IN ? AND version = 2", []string{"com.corex.ai.catalog.providers.read", "com.corex.ai.catalog.models.read"}).Find(&upgraded).Error)
	require.Len(t, upgraded, 4)
	for _, reg := range upgraded {
		var adapters []cm.AdapterEndpoint
		require.NoError(t, db.Where("registration_id = ?", reg.ID).Find(&adapters).Error)
		require.Len(t, adapters, 1)
		require.Equal(t, "core_internal", adapters[0].TransportType)
	}
	var count int64
	require.NoError(t, db.Model(&am.AuditEvent{}).Where("operation = ?", "UPGRADE_MEDIA_REGISTRATION").Count(&count).Error)
	require.EqualValues(t, 4, count)
	plan, err = runner.Plan(ctx, "plan")
	require.NoError(t, err)
	require.Empty(t, plan.Pending)
	require.NoError(t, db.Table("public.database_migration_records").Where("migration_id = ?", oldID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
