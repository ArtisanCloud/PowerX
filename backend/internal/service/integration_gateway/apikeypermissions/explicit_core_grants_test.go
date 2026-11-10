package apikeypermissions

import (
	"context"
	"encoding/json"
	"testing"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const explicitCoreTestCapability = "com.corex.ai.connection.test"

func coreGrantFixture(t *testing.T, db *gorm.DB) *iammodel.Permission {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}))
	p := &iammodel.Permission{Module: "ai", Resource: "connection", Action: "test", Source: platformPermissionSource, Status: iammodel.PermissionStatusActive, Effect: "allow", AllowAPIKey: true,
		Meta: datatypes.JSON([]byte(`{"type":"capability_service","capability_id":"com.corex.ai.connection.test","api_key_explicit":true,"core_internal_endpoint":"core://ai/connection","api_key":{"scope":"_scope.ai.connection.test","action":"test","resource_type":"api","resource_pattern":"ai-connection-test","effect":"allow"}}`))}
	require.NoError(t, db.Create(p).Error)
	require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: explicitCoreTestCapability, PluginID: "corex.platform", Title: "Connection test", Status: "published", CapabilitiesHash: "fixture", ProtocolHash: "fixture", Protocols: datatypes.JSON([]byte(`[{"channel":"core_internal","method":"INVOKE","endpoint":"core://ai/connection","actor_context":"service_actor","resource_scope":"tenant"}]`))}).Error)
	require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: explicitCoreTestCapability, TenantUUID: ownerTestTenant, Status: "published", Version: 1, ContractRef: "fixture"}).Error)
	return p
}

func TestCoreExplicitGrantDoesNotExpandSharedProfileAndSurvivesReconciliation(t *testing.T) {
	db, ctx, profile, key, ids := ownerFixture(t)
	p := coreGrantFixture(t, db)
	other := &gwmodel.IntegrationGatewayAPIKey{TenantUUID: ownerTestTenant, ProfileID: profile.ID, Name: "other", KeyHash: "core-other", Status: "active"}
	require.NoError(t, db.Create(other).Error)
	svc := NewAPIKeyOwnerService(db)
	for n := 0; n < 2; n++ {
		out, err := svc.AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{p.ID}, "root")
		require.NoError(t, err)
		require.Len(t, out.Grants, 1)
		require.Empty(t, out.Grants[0].PluginID)
	}
	stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, key.UUID)
	require.NoError(t, err)
	policy, err := ResolveOwnerPolicy(stored.PluginOwnerPolicy, nil)
	require.NoError(t, err)
	bound, err := BindOwnerPolicy(key.UUID, nil, policy)
	require.NoError(t, err)
	require.True(t, gwrepo.APIKeyPermissionGranted(bound, "_scope.ai.connection.test", "test", "api", "ai-connection-test"))
	profileIDs, err := iamrepo.NewAPIKeyProfilePermissionRepository(db).ListPermissionIDsOfProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, ids, profileIDs)
	rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(ctx, other.UUID)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	rows, err = gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(ctx, key.UUID)
	require.NoError(t, err)
	require.True(t, gwrepo.APIKeyPermissionGranted(rows, "_scope.ai.connection.test", "test", "api", "ai-connection-test"))
	_, err = svc.RevokeExplicitGrant(ctx, ownerTestTenant, key.UUID, p.ID, "root")
	require.NoError(t, err)
	rows, err = gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(ctx, key.UUID)
	require.NoError(t, err)
	require.False(t, gwrepo.APIKeyPermissionGranted(rows, "_scope.ai.connection.test", "test", "api", "ai-connection-test"))
}

func TestCoreExplicitGrantRejectsAdminInferredOrUnboundPermissions(t *testing.T) {
	for _, field := range []string{"type", "endpoint", "owner", "wildcard", "explicit", "protocol"} {
		t.Run(field, func(t *testing.T) {
			db, ctx, _, key, _ := ownerFixture(t)
			p := coreGrantFixture(t, db)
			var meta map[string]any
			require.NoError(t, json.Unmarshal(p.Meta, &meta))
			switch field {
			case "type":
				meta["type"] = "api"
			case "endpoint":
				meta["core_internal_endpoint"] = "/api/v1/admin/agents/test/connection"
			case "owner":
				meta["api_key"].(map[string]any)["plugin_id"] = "com.powerx.plugins.ecommerce"
			case "wildcard":
				meta["api_key"].(map[string]any)["resource_pattern"] = "*"
			case "explicit":
				meta["api_key_explicit"] = false
			case "protocol":
				require.NoError(t, db.Model(&capmodels.CapabilityRecord{}).Where("capability_id = ?", explicitCoreTestCapability).Update("protocols", datatypes.JSON([]byte(`[]`))).Error)
			}
			raw, err := json.Marshal(meta)
			require.NoError(t, err)
			require.NoError(t, db.Model(p).Update("meta", datatypes.JSON(raw)).Error)
			_, err = NewAPIKeyOwnerService(db).AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{p.ID}, "root")
			require.Error(t, err)
			rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(context.Background(), key.UUID)
			require.NoError(t, err)
			require.False(t, gwrepo.APIKeyPermissionGranted(rows, "_scope.ai.connection.test", "test", "api", "ai-connection-test"))
		})
	}
}
