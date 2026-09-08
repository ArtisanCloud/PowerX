package plugin

import (
	"context"
	"encoding/json"
	"testing"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIndependentApprovalSurvivesManifestRemovalAndIsExplicitlyRevocable(t *testing.T) {
	db := newPluginDrainTestDB(t)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}))
	const capability = "com.corex.iam.members.read"
	const pluginID = "com.powerx.plugins.test"
	seedPublishedTenantCapability(t, db, tenantCapabilityGrantTestUUID, capability)
	svc := NewTenantPluginInstanceService(db)
	_, _, _, err := svc.Enable(context.Background(), tenantCapabilityGrantTestUUID, plugin_mgr.Plugin{ID: pluginID, RequiredCapabilities: []string{capability}}, nil)
	require.NoError(t, err)
	admin := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserUUID: uuid.NewString()})
	for i := 0; i < 2; i++ {
		require.NoError(t, svc.SetIndependentCapabilityApproval(admin, tenantCapabilityGrantTestUUID, pluginID, capability, true))
	}
	var approvals []setting.PluginCapabilityApproval
	require.NoError(t, db.Find(&approvals).Error)
	require.Len(t, approvals, 1)
	firstApproval := approvals[0].UUID
	manifest := plugin_mgr.Manifest{ID: pluginID}
	require.NoError(t, svc.SyncManifestRequiredCapabilities(context.Background(), manifest))
	readAllowed := func() []string {
		cfg, err := settingrepo.NewPluginInstanceConfigRepository(db).Get(context.Background(), tenantCapabilityGrantTestUUID, pluginID, settingrepo.KeyClientCredentials)
		require.NoError(t, err)
		var payload struct {
			Allowed []string `json:"allowed_capabilities"`
		}
		require.NoError(t, json.Unmarshal(cfg.ValueJSON, &payload))
		return payload.Allowed
	}
	require.Equal(t, []string{capability}, readAllowed())
	for i := 0; i < 2; i++ {
		require.NoError(t, svc.SetIndependentCapabilityApproval(admin, tenantCapabilityGrantTestUUID, pluginID, capability, false))
	}
	require.Empty(t, readAllowed())
	require.NoError(t, svc.SetIndependentCapabilityApproval(admin, tenantCapabilityGrantTestUUID, pluginID, capability, true))
	approvals = nil
	require.NoError(t, db.Order("created_at ASC").Find(&approvals).Error)
	require.Len(t, approvals, 2)
	require.Equal(t, "revoked", approvals[0].Status)
	require.NotNil(t, approvals[0].RevokedAt)
	require.Equal(t, firstApproval, approvals[0].UUID)
	require.NotEqual(t, firstApproval, approvals[1].UUID)
	require.Equal(t, []string{capability}, readAllowed())
	require.NoError(t, db.Create(&capmodels.CapabilityRegistration{TenantUUID: tenantCapabilityGrantTestUUID, CapabilityID: capability, Status: "disabled", ContractRef: "test", Version: 99, RoutingPolicyID: uuid.New()}).Error)
	require.ErrorContains(t, svc.SetIndependentCapabilityApproval(admin, tenantCapabilityGrantTestUUID, pluginID, capability, true), "plugin.capability_approval_registration_inactive")
	// Revocation remains possible even after the capability registration is withdrawn.
	require.NoError(t, svc.SetIndependentCapabilityApproval(admin, tenantCapabilityGrantTestUUID, pluginID, capability, false))
	require.Empty(t, readAllowed())
}

func TestIndependentApprovalRejectsPluginAndNonAdmin(t *testing.T) {
	svc := NewTenantPluginInstanceService(newPluginDrainTestDB(t))
	for _, claims := range []*reqctx.CoreXClaims{nil, {UserUUID: uuid.NewString()}, {IsRoot: true, PluginID: "com.powerx.plugins.test", UserUUID: uuid.NewString()}, {IsRoot: true}} {
		ctx := context.Background()
		if claims != nil {
			ctx = reqctx.WithClaims(ctx, claims)
		}
		require.ErrorContains(t, svc.SetIndependentCapabilityApproval(ctx, tenantCapabilityGrantTestUUID, "plugin", "capability", true), "plugin.capability_approval_forbidden")
	}
}
