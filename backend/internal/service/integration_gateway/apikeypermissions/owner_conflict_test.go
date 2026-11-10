package apikeypermissions

import (
	"encoding/json"
	"testing"

	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func conflictPermission(t *testing.T, db *gorm.DB, resource string) iammodel.Permission {
	t.Helper()
	meta, err := json.Marshal(map[string]any{"label": "电商套餐读取 " + resource, "capability_id": "com.powerx.plugins.ecommerce." + resource + ".read", "api_key": map[string]any{"scope": "_scope.ecommerce." + resource + ".read", "action": "read", "resource_type": "capability", "resource_pattern": resource, "plugin_id": "com.powerx.plugins.ecommerce", "effect": "allow"}})
	require.NoError(t, err)
	p := iammodel.Permission{Module: "commerce", Resource: resource, Action: "read", AllowAPIKey: true, Status: iammodel.PermissionStatusActive, Meta: datatypes.JSON(meta)}
	require.NoError(t, db.Create(&p).Error)
	return p
}

func TestProfileOwnerConflictsCollectAllKeysAndRollBack(t *testing.T) {
	db, ctx, profile, legacy, originalIDs := ownerFixture(t)
	first := conflictPermission(t, db, "plans")
	second := conflictPermission(t, db, "plan_details")
	repo := gwrepo.NewIntegrationGatewayAPIKeyRepository(db)
	keys := []gwmodel.IntegrationGatewayAPIKey{*legacy}
	for _, in := range []struct{ name, status, tenant, owner string }{{"crm-key", "active", ownerTestTenant, "com.powerx.plugins.crm"}, {"matching-key", "active", ownerTestTenant, "com.powerx.plugins.ecommerce"}, {"revoked-key", "revoked", ownerTestTenant, ""}, {"foreign-key", "active", uuid.NewString(), ""}} {
		policyOwners := []string{}
		if in.owner != "" {
			policyOwners = append(policyOwners, in.owner)
		}
		key, err := repo.Create(ctx, &gwmodel.IntegrationGatewayAPIKey{TenantUUID: in.tenant, ProfileID: profile.ID, Name: in.name, KeyPrefix: "do-not-return-prefix", KeyHash: uuid.NewString(), Status: in.status, PluginOwnerPolicy: EncodePluginOwners(policyOwners)})
		require.NoError(t, err)
		keys = append(keys, *key)
	}
	before := map[uuid.UUID][]gwmodel.IntegrationGatewayAPIKeyPermission{}
	permRepo := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db)
	for _, key := range keys {
		rows, err := permRepo.ListByAPIKeyUUID(ctx, key.UUID)
		require.NoError(t, err)
		before[key.UUID] = rows
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(tx).GrantByIDsTx(tx, profile.ID, []uint64{first.ID, second.ID}))
		_, _, err := NewAPIKeyOwnerService(tx).SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	})
	require.Equal(t, 400, dto.StatusCode(err))
	require.Equal(t, "API_KEY_PLUGIN_OWNER_CONFLICT", dto.CodeOf(err))
	details := dto.DetailsOf(err)
	require.Equal(t, profile.ID, details["profile_id"])
	require.Equal(t, profile.Name, details["profile_name"])
	require.Equal(t, 4, details["conflict_count"])
	require.Equal(t, 2, details["conflicting_key_count"])
	conflicts := details["conflicts"].([]OwnerPermissionConflict)
	require.Len(t, conflicts, 4)
	names := map[string]bool{}
	for _, c := range conflicts {
		names[c.KeyName] = true
		require.Contains(t, []uint64{first.ID, second.ID}, c.PermissionID)
		require.Equal(t, "com.powerx.plugins.ecommerce", c.RequiredPluginID)
		require.NotEmpty(t, c.PermissionName)
		require.NotEmpty(t, c.CapabilityID)
		require.NotEmpty(t, c.OwnerPluginIDs)
		require.NotContains(t, c.PermissionOwnerPluginIDs, c.RequiredPluginID)
	}
	require.Equal(t, map[string]bool{"legacy-key": true, "crm-key": true}, names)
	raw, err := json.Marshal(details)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "do-not-return-prefix")
	for _, key := range keys {
		stored, err := repo.GetByUUID(ctx, key.UUID)
		require.NoError(t, err)
		require.Equal(t, key.PluginOwnerPolicy, stored.PluginOwnerPolicy)
		rows, err := permRepo.ListByAPIKeyUUID(ctx, key.UUID)
		require.NoError(t, err)
		require.Equal(t, before[key.UUID], rows)
		require.NotContains(t, string(raw), key.KeyHash)
	}
	ids, err := iamrepo.NewAPIKeyProfilePermissionRepository(db).ListPermissionIDsOfProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, originalIDs, ids)
}

func TestClearingOwnerStillConflictsWithFixedPluginPermission(t *testing.T) {
	db, ctx, profile, key, _ := ownerFixture(t)
	p := conflictPermission(t, db, "plans")
	require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(db).GrantByIDsTx(db, profile.ID, []uint64{p.ID}))
	original := EncodePluginOwners([]string{"com.powerx.plugins.ecommerce"})
	require.NoError(t, db.Model(key).Update("plugin_owner_policy", original).Error)
	_, err := NewAPIKeyOwnerService(db).SetOwners(ctx, ownerTestTenant, key.UUID, []string{}, "root")
	require.Equal(t, 400, dto.StatusCode(err))
	require.Equal(t, "API_KEY_PLUGIN_OWNER_CONFLICT", dto.CodeOf(err))
	conflicts := dto.DetailsOf(err)["conflicts"].([]OwnerPermissionConflict)
	require.Len(t, conflicts, 1)
	require.Equal(t, key.UUID.String(), conflicts[0].KeyID)
	require.Equal(t, p.ID, conflicts[0].PermissionID)
	require.Empty(t, conflicts[0].OwnerPluginIDs)
	stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, key.UUID)
	require.NoError(t, err)
	require.Equal(t, original, stored.PluginOwnerPolicy)
}

func TestLegacyOwnerConflictReportsPermissionSpecificScope(t *testing.T) {
	key := gwmodel.IntegrationGatewayAPIKey{Name: "legacy"}
	key.UUID = uuid.New()
	policy := OwnerPolicy{Mode: "legacy_permissions", Bindings: []OwnerPermissionBinding{{Scope: "other", Action: "read", ResourceType: "api", ResourcePattern: "tasks", PluginID: "com.powerx.plugins.ecommerce", Effect: "allow"}}}
	definition := profileGrantDefinition{permissionID: 1, grant: gwmodel.IntegrationGatewayAPIKeyPermission{Scope: "plans", Action: "read", ResourceType: "capability", ResourcePattern: "plans", PluginID: "com.powerx.plugins.ecommerce", Effect: "allow"}}
	report := ownerConflictReport{}
	report.add(key, policy, []profileGrantDefinition{definition})
	require.Equal(t, 1, report.count)
	require.Equal(t, []string{"com.powerx.plugins.ecommerce"}, report.conflicts[0].OwnerPluginIDs)
	require.Empty(t, report.conflicts[0].PermissionOwnerPluginIDs)
	_, err := BindOwnerPolicy(key.UUID, []gwmodel.IntegrationGatewayAPIKeyPermission{definition.grant}, policy)
	require.Equal(t, "API_KEY_PLUGIN_OWNER_CONFLICT", dto.CodeOf(err))
}

func TestOwnerConflictDetailsAreBoundedWithTruthfulCounts(t *testing.T) {
	key := gwmodel.IntegrationGatewayAPIKey{Name: "unbound"}
	key.UUID = uuid.New()
	definitions := make([]profileGrantDefinition, maxOwnerConflictDetails+3)
	for i := range definitions {
		definitions[i] = profileGrantDefinition{permissionID: uint64(i + 1), grant: gwmodel.IntegrationGatewayAPIKeyPermission{PluginID: "com.example.plugin"}}
	}
	report := ownerConflictReport{}
	report.add(key, OwnerPolicy{Mode: "key"}, definitions)
	details := dto.DetailsOf(report.err(1, "fixture"))
	require.Equal(t, maxOwnerConflictDetails+3, details["conflict_count"])
	require.Equal(t, true, details["truncated"])
	require.Len(t, details["conflicts"], maxOwnerConflictDetails)
}
