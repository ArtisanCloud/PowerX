package plugin

import (
	"context"
	"encoding/json"
	"testing"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const tenantCapabilityGrantTestUUID = "6b5d0240-9920-46da-b707-88200e0f51ea"

func TestTenantPluginEnableMergesManifestRequiredCapabilities(t *testing.T) {
	db := newPluginDrainTestDB(t)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}))
	seedPublishedTenantCapability(t, db, tenantCapabilityGrantTestUUID, "com.corex.iam.members.read")

	svc := NewTenantPluginInstanceService(db)
	_, _, _, err := svc.Enable(context.Background(), tenantCapabilityGrantTestUUID, plugin_mgr.Plugin{
		ID:                   "com.powerx.plugin.ai-craft",
		Version:              "0.1.74",
		RequiredCapabilities: []string{"com.corex.iam.members.read"},
	}, nil)
	require.NoError(t, err)

	cfg, err := reposetting.NewPluginInstanceConfigRepository(db).Get(context.Background(), tenantCapabilityGrantTestUUID, "com.powerx.plugin.ai-craft", reposetting.KeyClientCredentials)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	var doc struct {
		AllowedCapabilities []string `json:"allowed_capabilities"`
	}
	require.NoError(t, json.Unmarshal(cfg.ValueJSON, &doc))
	require.Equal(t, []string{"com.corex.iam.members.read"}, doc.AllowedCapabilities)
}

func TestSyncManifestRequiredCapabilitiesPreservesExistingGrants(t *testing.T) {
	db := newPluginDrainTestDB(t)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}))
	seedPublishedTenantCapability(t, db, tenantCapabilityGrantTestUUID, "com.corex.iam.members.read")
	repo := reposetting.NewPluginInstanceConfigRepository(db)
	require.NoError(t, repo.Upsert(context.Background(), &dbsetting.PluginInstanceConfig{
		TenantUUID: tenantCapabilityGrantTestUUID,
		PluginID:   "com.powerx.plugin.ai-craft",
		Key:        reposetting.KeyClientCredentials,
		ValueJSON:  datatypes.JSON([]byte(`{"client_id":"existing","allowed_capabilities":["com.corex.existing"]}`)),
		Enabled:    true,
	}))

	svc := NewTenantPluginInstanceService(db)
	require.NoError(t, svc.SyncManifestRequiredCapabilities(context.Background(), plugin_mgr.Manifest{
		ID:      "com.powerx.plugin.ai-craft",
		Version: "0.1.74",
		Capabilities: plugin_mgr.HostCapabilitySpec{Required: []string{
			"com.corex.iam.members.read",
		}},
	}))

	cfg, err := repo.Get(context.Background(), tenantCapabilityGrantTestUUID, "com.powerx.plugin.ai-craft", reposetting.KeyClientCredentials)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(cfg.ValueJSON, &doc))
	require.Equal(t, "existing", doc["client_id"])
	require.ElementsMatch(t, []any{"com.corex.existing", "com.corex.iam.members.read"}, doc["allowed_capabilities"])
}

func TestSyncManifestRequiredCapabilitiesRejectsUnregisteredCapability(t *testing.T) {
	db := newPluginDrainTestDB(t)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}))
	require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: "com.corex.iam.members.read", PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
	repo := reposetting.NewPluginInstanceConfigRepository(db)
	require.NoError(t, repo.Upsert(context.Background(), &dbsetting.PluginInstanceConfig{TenantUUID: tenantCapabilityGrantTestUUID, PluginID: "com.powerx.plugin.ai-craft", Key: reposetting.KeyClientCredentials, ValueJSON: datatypes.JSON([]byte(`{"client_id":"existing"}`)), Enabled: true}))

	err := NewTenantPluginInstanceService(db).SyncManifestRequiredCapabilities(context.Background(), plugin_mgr.Manifest{ID: "com.powerx.plugin.ai-craft", Capabilities: plugin_mgr.HostCapabilitySpec{Required: []string{"com.corex.iam.members.read"}}})
	require.ErrorContains(t, err, "not registered for tenant")
}

func seedPublishedTenantCapability(t *testing.T, db *gorm.DB, tenantUUID, capabilityID string) {
	t.Helper()
	require.NoError(t, db.Create(&capmodels.CapabilityRecord{
		CapabilityID: capabilityID, PluginID: "core", PluginVersion: "1", Title: "test",
		CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published",
	}).Error)
	require.NoError(t, db.Create(&capmodels.CapabilityRegistration{
		CapabilityID: capabilityID, TenantUUID: tenantUUID, ContractRef: "test", Status: "published", Version: 1,
	}).Error)
}
