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
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const explicitTestPlugin = "com.powerx.plugins.ecommerce"
const explicitTestCapability = explicitTestPlugin + ".product.spu.subscription.plan.read"

func explicitPermissionFixture(t *testing.T, db *gorm.DB) *iammodel.Permission {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}))
	spec := &pluginpermission.APIKeySpec{CapabilityID: explicitTestCapability, PluginID: explicitTestPlugin, Scope: "_scope.ecommerce.subscription_plans.read", Action: "read", ResourceType: "capability", ResourcePattern: "subscription_plans", Effect: "allow"}
	meta, err := pluginpermission.ExplicitAPIKeyMetadata(spec, explicitTestPlugin, explicitTestCapability, true)
	require.NoError(t, err)
	raw, err := json.Marshal(meta)
	require.NoError(t, err)
	p := &iammodel.Permission{Module: "commerce", Resource: "subscription_plans_service", Action: "read", Source: "plugin:" + explicitTestPlugin, Status: iammodel.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON(raw)}
	require.NoError(t, db.Create(p).Error)
	require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: explicitTestCapability, PluginID: explicitTestPlugin, PluginVersion: "0.3.1", Title: "Read plans", Status: "published", CapabilitiesHash: "fixture", ProtocolHash: "fixture"}).Error)
	require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: explicitTestCapability, TenantUUID: ownerTestTenant, Status: "published", Version: 1, ContractRef: "2.0.0"}).Error)
	return p
}

func explicitAllowed(t *testing.T, db *gorm.DB, keyID uuid.UUID) bool {
	t.Helper()
	rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(context.Background(), keyID)
	require.NoError(t, err)
	return gwrepo.APIKeyPermissionGrantedForPlugin(rows, explicitTestPlugin, "_scope.ecommerce.subscription_plans.read", "read", "capability", "subscription_plans")
}

func TestExplicitGrantChangesOnlySelectedKey(t *testing.T) {
	db, ctx, profile, key, ids := ownerFixture(t)
	p := explicitPermissionFixture(t, db)
	svc := NewAPIKeyOwnerService(db)
	other, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).Create(ctx, &gwmodel.IntegrationGatewayAPIKey{TenantUUID: ownerTestTenant, ProfileID: profile.ID, Name: "other-key", KeyHash: uuid.NewString(), Status: "active"})
	require.NoError(t, err)
	beforeHash := key.KeyHash
	for i := 0; i < 2; i++ {
		result, err := svc.AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{p.ID, p.ID}, "root")
		require.NoError(t, err)
		require.Len(t, result.Grants, 1)
		require.Equal(t, explicitTestCapability, result.Grants[0].CapabilityID)
	}
	require.True(t, explicitAllowed(t, db, key.UUID))
	require.False(t, explicitAllowed(t, db, other.UUID))
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.scrm"))
	stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, key.UUID)
	require.NoError(t, err)
	require.Equal(t, beforeHash, stored.KeyHash)
	require.Equal(t, profile.ID, stored.ProfileID)
	policy, err := ResolveOwnerPolicy(stored.PluginOwnerPolicy, nil)
	require.NoError(t, err)
	require.Equal(t, 2, policy.Version)
	profileIDs, err := iamrepo.NewAPIKeyProfilePermissionRepository(db).ListPermissionIDsOfProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, ids, profileIDs)
	rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(ctx, key.UUID)
	require.NoError(t, err)
	require.Len(t, rows, 3, "two Profile grants plus one independent grant")
	var audits int64
	require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyAuditLog{}).Where("api_key_uuid = ?", key.UUID).Count(&audits).Error)
	require.EqualValues(t, 2, audits)
}

func TestExplicitGrantSurvivesProfileAndOwnerReconciliation(t *testing.T) {
	db, ctx, profile, key, ids := ownerFixture(t)
	p := explicitPermissionFixture(t, db)
	svc := NewAPIKeyOwnerService(db)
	_, err := svc.AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{p.ID}, "root")
	require.NoError(t, err)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := iamrepo.NewAPIKeyProfilePermissionRepository(tx).RevokeByIDsTx(tx, profile.ID, ids); err != nil {
			return err
		}
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	require.True(t, explicitAllowed(t, db, key.UUID))
	_, err = svc.SetOwners(ctx, ownerTestTenant, key.UUID, []string{"com.powerx.plugins.crm"}, "root")
	require.NoError(t, err)
	require.True(t, explicitAllowed(t, db, key.UUID))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := iamrepo.NewAPIKeyProfilePermissionRepository(tx).GrantByIDsTx(tx, profile.ID, ids); err != nil {
			return err
		}
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	require.True(t, explicitAllowed(t, db, key.UUID))
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.crm"))
	stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, key.UUID)
	require.NoError(t, err)
	policy, err := ResolveOwnerPolicy(stored.PluginOwnerPolicy, nil)
	require.NoError(t, err)
	rotated, err := BindOwnerPolicy(uuid.New(), nil, policy)
	require.NoError(t, err)
	require.Len(t, rotated, 1, "rotation keeps the independent grant even with an empty Profile")
	_, err = svc.RevokeExplicitGrant(ctx, ownerTestTenant, key.UUID, p.ID, "root")
	require.NoError(t, err)
	require.False(t, explicitAllowed(t, db, key.UUID))
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.crm"))
}

func TestExplicitGrantRejectsInvalidBatchWithoutPartialWrite(t *testing.T) {
	db, ctx, _, key, ids := ownerFixture(t)
	p := explicitPermissionFixture(t, db)
	svc := NewAPIKeyOwnerService(db)
	_, err := svc.AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{p.ID, ids[0]}, "root")
	require.Error(t, err, "inferred Profile grants are not independent explicit plugin declarations")
	require.False(t, explicitAllowed(t, db, key.UUID))
	stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, key.UUID)
	require.NoError(t, err)
	require.Empty(t, stored.PluginOwnerPolicy)
	_, err = svc.AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{0}, "root")
	require.Error(t, err)
	require.NoError(t, db.Model(&capmodels.CapabilityRegistration{}).Where("capability_id = ?", explicitTestCapability).Update("status", "disabled").Error)
	_, err = svc.AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{p.ID}, "root")
	require.ErrorContains(t, err, "CAPABILITY_TENANT_NOT_REGISTERED")
	require.False(t, explicitAllowed(t, db, key.UUID))
}

func TestExplicitGrantRequiresTenantAdministratorAndLiveDeclaration(t *testing.T) {
	db, ctx, _, key, _ := ownerFixture(t)
	p := explicitPermissionFixture(t, db)
	svc := NewAPIKeyOwnerService(db)
	sts := &reqctx.CoreXClaims{TenantUUID: ownerTestTenant}
	sts.Issuer = "powerx-sts"
	for _, bad := range []context.Context{
		reqctx.WithAuthenticatedAPIKeyHash(ctx, "consumer-key"),
		reqctx.WithClaims(ctx, sts),
		reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 3, TenantUUID: ownerTestTenant, Roles: []string{"role_user"}}),
		reqctx.WithTenantUUID(ctx, uuid.NewString()),
	} {
		_, err := svc.AppendExplicitGrants(bad, ownerTestTenant, key.UUID, []uint64{p.ID}, "bad")
		require.Error(t, err)
	}
	require.NoError(t, db.Model(p).Update("allow_api_key", false).Error)
	_, err := svc.AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{p.ID}, "root")
	require.Error(t, err)
	require.NoError(t, db.Model(p).Update("allow_api_key", true).Error)
	require.NoError(t, db.Model(&capmodels.CapabilityRecord{}).Where("capability_id = ?", explicitTestCapability).Update("status", "draft").Error)
	_, err = svc.AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{p.ID}, "root")
	require.ErrorContains(t, err, "CAPABILITY_UNKNOWN")
	require.False(t, explicitAllowed(t, db, key.UUID))
}

func TestKeyOnlyGrantDoesNotApplyUnrelatedProfileChanges(t *testing.T) {
	db, ctx, profile, key, _ := ownerFixture(t)
	p := explicitPermissionFixture(t, db)
	meta, err := json.Marshal(map[string]any{"api_key": map[string]any{"scope": "_scope.unrelated.read", "action": "read", "resource_type": "api", "resource_pattern": "unrelated", "effect": "allow"}})
	require.NoError(t, err)
	unrelated := &iammodel.Permission{Module: "other", Resource: "unrelated", Action: "read", Status: iammodel.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON(meta)}
	require.NoError(t, db.Create(unrelated).Error)
	require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(db).GrantByIDsTx(db, profile.ID, []uint64{unrelated.ID}))
	_, err = NewAPIKeyOwnerService(db).AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{p.ID}, "root")
	require.NoError(t, err)
	rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(ctx, key.UUID)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.False(t, gwrepo.APIKeyPermissionGranted(rows, "_scope.unrelated.read", "read", "api", "unrelated"))
}
