package plugin

import (
	"context"
	"encoding/json"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	metaservice "github.com/ArtisanCloud/PowerX/internal/service/metadata"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
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

func TestSyncManifestRequiredCapabilitiesQuarantinesUnattributedGrants(t *testing.T) {
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
	require.ElementsMatch(t, []any{"com.corex.iam.members.read"}, doc["allowed_capabilities"])
	require.ElementsMatch(t, []any{"com.corex.existing"}, doc["unattributed_capabilities"])
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

func TestSyncManifestRequiredCapabilitiesRevokesRemovedAndEmptyRequired(t *testing.T) {
	db := newPluginDrainTestDB(t)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}))
	const first = "com.corex.iam.members.read"
	const second = "com.corex.agent.session.manage"
	seedPublishedTenantCapability(t, db, tenantCapabilityGrantTestUUID, first)
	seedPublishedTenantCapability(t, db, tenantCapabilityGrantTestUUID, second)
	repo := reposetting.NewPluginInstanceConfigRepository(db)
	const pluginID = "com.powerx.plugin.grant-contract"
	require.NoError(t, repo.Upsert(context.Background(), &dbsetting.PluginInstanceConfig{
		TenantUUID: tenantCapabilityGrantTestUUID, PluginID: pluginID,
		Key: reposetting.KeyClientCredentials, Enabled: true,
		ValueJSON: datatypes.JSON([]byte(`{"client_id":"existing","client_secret_hash":"preserved"}`)),
	}))
	svc := NewTenantPluginInstanceService(db)
	for _, required := range [][]string{{first, second}, {first}, {}, {}} {
		require.NoError(t, svc.SyncManifestRequiredCapabilities(context.Background(), plugin_mgr.Manifest{
			ID: pluginID, Capabilities: plugin_mgr.HostCapabilitySpec{Required: required},
		}))
		cfg, err := repo.Get(context.Background(), tenantCapabilityGrantTestUUID, pluginID, reposetting.KeyClientCredentials)
		require.NoError(t, err)
		var doc struct {
			Allowed    []string `json:"allowed_capabilities"`
			Manifest   []string `json:"manifest_capabilities"`
			SecretHash string   `json:"client_secret_hash"`
		}
		require.NoError(t, json.Unmarshal(cfg.ValueJSON, &doc))
		require.ElementsMatch(t, required, doc.Allowed)
		require.ElementsMatch(t, required, doc.Manifest)
		require.Equal(t, "preserved", doc.SecretHash)
	}
}

// Reuse the same authenticated context before and after synchronization: an
// already-issued token must not retain permissions removed from credentials.
func TestManifestRevocationChangesGrantStatusAndHostAuthorization(t *testing.T) {
	db := newPluginDrainTestDB(t)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}))
	const capability = "com.corex.metadata.dictionary.read"
	for _, id := range []string{capability, capservice.GrantStatusCapabilityID} {
		seedPublishedTenantCapability(t, db, tenantCapabilityGrantTestUUID, id)
	}
	const pluginID = "com.powerx.plugin.revocation-contract"
	repo := reposetting.NewPluginInstanceConfigRepository(db)
	require.NoError(t, repo.Upsert(context.Background(), &dbsetting.PluginInstanceConfig{TenantUUID: tenantCapabilityGrantTestUUID, PluginID: pluginID, Key: reposetting.KeyClientCredentials, Enabled: true, ValueJSON: datatypes.JSON([]byte(`{"client_id":"existing"}`))}))
	svc := NewTenantPluginInstanceService(db)
	sync := func(required []string) {
		require.NoError(t, svc.SyncManifestRequiredCapabilities(context.Background(), plugin_mgr.Manifest{ID: pluginID, Capabilities: plugin_mgr.HostCapabilitySpec{Required: required}}))
	}
	sync([]string{capability, capservice.GrantStatusCapabilityID})
	claims := &reqctx.CoreXClaims{TenantUUID: tenantCapabilityGrantTestUUID, PluginID: pluginID, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: []string{"powerx:api"}}}
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenantCapabilityGrantTestUUID), claims)
	statusService := capservice.NewGrantStatusService(db)
	access := metaservice.NewHostContractAccess(db)
	items, err := statusService.CheckCurrentCredential(ctx, []string{capability})
	require.NoError(t, err)
	require.Equal(t, capservice.GrantStatusGranted, items[0].Status)
	_, err = access.Authorize(ctx, "", "dictionary", "read")
	require.NoError(t, err)
	sync([]string{capservice.GrantStatusCapabilityID})
	items, err = statusService.CheckCurrentCredential(ctx, []string{capability})
	require.NoError(t, err)
	require.Equal(t, capservice.GrantStatusNotGranted, items[0].Status)
	_, err = access.Authorize(ctx, "", "dictionary", "read")
	require.Equal(t, http.StatusForbidden, dto.StatusCode(err))
	require.Equal(t, "METADATA_FORBIDDEN", dto.CodeOf(err))
}
