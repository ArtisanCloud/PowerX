package integration_gateway

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUpgradeCatalogPreservesExistingCapabilityIdentityStateAndGraphs(t *testing.T) {
	dsn := os.Getenv("POWERX_UPGRADE_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	var name string
	require.NoError(t, db.Raw("select current_database()").Scan(&name).Error)
	require.True(t, strings.HasPrefix(name, "powerx_upgrade_test_"))
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS public.capability_registry_records").Error)
	require.NoError(t, db.AutoMigrate(&models.CapabilityRecord{}))
	defs := platformCapabilityDefinitions()
	require.NotEmpty(t, defs)
	old := defs[0].toRecord(PlatformPluginID, PlatformPluginVersion, time.Now().UTC())
	old.Status = "disabled"
	old.CreatedBy = "retained-operator"
	old.CompositeGraphs = datatypes.JSON(`[{"retained":true}]`)
	old.WorkflowTemplateRefs = datatypes.JSON(`["retained-template"]`)
	require.NoError(t, db.Create(old).Error)
	seeder := NewBaseCapabilitySeeder(BaseCapabilitySeederOptions{RecordRepo: repo.NewCapabilityRecordRepository(db, nil), PreserveInstalledState: true})
	require.NoError(t, seeder.Ensure(context.Background()))
	var got models.CapabilityRecord
	require.NoError(t, db.Where("capability_id = ?", old.CapabilityID).First(&got).Error)
	require.Equal(t, old.ID, got.ID)
	require.Equal(t, old.UUID, got.UUID)
	require.Equal(t, "disabled", got.Status)
	require.Equal(t, old.CompositeGraphs, got.CompositeGraphs)
	require.Equal(t, old.WorkflowTemplateRefs, got.WorkflowTemplateRefs)
	require.Equal(t, "retained-operator", got.CreatedBy)
	got.PluginID = "another-owner"
	require.NoError(t, db.Save(&got).Error)
	require.ErrorContains(t, seeder.Ensure(context.Background()), "UPGRADE_CAPABILITY_SOURCE_CONFLICT")
}
