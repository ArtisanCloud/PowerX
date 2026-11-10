package apikeypermissions

import (
	"errors"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestProfilePluginServiceGrantsPreserveCallerResourceOwners(t *testing.T) {
	db, ctx, profile, key, originalIDs := ownerFixture(t)
	permission := explicitPermissionFixture(t, db)
	svc := NewAPIKeyOwnerService(db)
	keys := []*gwmodel.IntegrationGatewayAPIKey{key}
	for _, owner := range []string{"com.powerx.plugins.crm", "com.powerx.plugins.workspacex", ""} {
		ids := []string{}
		if owner != "" {
			ids = append(ids, owner)
		}
		k, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).Create(ctx, &gwmodel.IntegrationGatewayAPIKey{TenantUUID: ownerTestTenant, ProfileID: profile.ID, Name: owner, KeyHash: uuid.NewString(), Status: "active", PluginOwnerPolicy: EncodePluginOwners(ids)})
		require.NoError(t, err)
		keys = append(keys, k)
	}
	unselected, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).Create(ctx, &gwmodel.IntegrationGatewayAPIKey{TenantUUID: ownerTestTenant, ProfileID: profile.ID + 99, Name: "other-profile", KeyHash: uuid.NewString(), Status: "active"})
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(tx).GrantByIDsTx(tx, profile.ID, []uint64{permission.ID}))
			_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
			return err
		}))
	}
	for _, k := range keys {
		require.True(t, explicitAllowed(t, db, k.UUID))
		stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, k.UUID)
		require.NoError(t, err)
		policy, err := ResolveOwnerPolicy(stored.PluginOwnerPolicy, nil)
		require.NoError(t, err)
		require.Empty(t, policy.ExplicitGrants, "Profile grant must remain owned by Profile, not copied to per-Key overrides")
		require.NotContains(t, policy.Owners(), explicitTestPlugin)
		require.Equal(t, k.KeyHash, stored.KeyHash)
		rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(ctx, k.UUID)
		require.NoError(t, err)
		count := 0
		for _, row := range rows {
			if row.Scope == "_scope.ecommerce.subscription_plans.read" {
				count++
				require.Equal(t, explicitTestPlugin, row.PluginID)
				require.NotEqual(t, "manage", row.Action)
			}
		}
		require.Equal(t, 1, count)
	}
	require.False(t, explicitAllowed(t, db, unselected.UUID))
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.scrm"))
	require.False(t, ownerAllows(t, db, key.UUID, explicitTestPlugin))
	_, err = svc.SetOwners(ctx, ownerTestTenant, key.UUID, []string{"com.powerx.plugins.crm"}, "root")
	require.NoError(t, err)
	require.True(t, explicitAllowed(t, db, key.UUID))
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.crm"))
	require.False(t, ownerAllows(t, db, key.UUID, explicitTestPlugin))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(tx).RevokeByIDsTx(tx, profile.ID, []uint64{permission.ID}))
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	for _, k := range keys {
		require.False(t, explicitAllowed(t, db, k.UUID))
	}
	ids, err := iamrepo.NewAPIKeyProfilePermissionRepository(db).ListPermissionIDsOfProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, originalIDs, ids)
}

func TestProfileProviderDeclarationFailuresRollBackProfileAndAllKeys(t *testing.T) {
	for _, kind := range []string{"unpublished", "unregistered", "malformed-provider", "foreign-tenant"} {
		t.Run(kind, func(t *testing.T) {
			db, ctx, profile, key, original := ownerFixture(t)
			p := explicitPermissionFixture(t, db)
			switch kind {
			case "unpublished":
				require.NoError(t, db.Model(&capmodels.CapabilityRecord{}).Where("capability_id = ?", explicitTestCapability).Update("status", "draft").Error)
			case "unregistered":
				require.NoError(t, db.Model(&capmodels.CapabilityRegistration{}).Where("capability_id = ?", explicitTestCapability).Update("status", "disabled").Error)
			case "malformed-provider":
				require.NoError(t, db.Model(p).Update("source", "plugin:com.powerx.plugins.other").Error)
			case "foreign-tenant":
				require.NoError(t, db.Model(&capmodels.CapabilityRegistration{}).Where("capability_id = ?", explicitTestCapability).Update("tenant_uuid", uuid.NewString()).Error)
			}
			err := db.Transaction(func(tx *gorm.DB) error {
				if err := iamrepo.NewAPIKeyProfilePermissionRepository(tx).GrantByIDsTx(tx, profile.ID, []uint64{p.ID}); err != nil {
					return err
				}
				_, _, err := NewAPIKeyOwnerService(db).SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
				return err
			})
			require.Error(t, err)
			require.False(t, explicitAllowed(t, db, key.UUID))
			require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.scrm"))
			stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, key.UUID)
			require.NoError(t, err)
			require.Equal(t, key.PluginOwnerPolicy, stored.PluginOwnerPolicy)
			ids, err := iamrepo.NewAPIKeyProfilePermissionRepository(db).ListPermissionIDsOfProfile(ctx, profile.ID)
			require.NoError(t, err)
			require.ElementsMatch(t, original, ids)
		})
	}
}

func TestProfileUncheckPreservesExplicitKeyExceptionAndOnlyThatException(t *testing.T) {
	db, ctx, profile, key, _ := ownerFixture(t)
	p := explicitPermissionFixture(t, db)
	svc := NewAPIKeyOwnerService(db)
	require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(db).GrantByIDsTx(db, profile.ID, []uint64{p.ID}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	_, err := svc.AppendExplicitGrants(ctx, ownerTestTenant, key.UUID, []uint64{p.ID}, "root")
	require.NoError(t, err)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(tx).RevokeByIDsTx(tx, profile.ID, []uint64{p.ID}))
		_, _, err := svc.SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	}))
	require.True(t, explicitAllowed(t, db, key.UUID))
	_, err = svc.RevokeExplicitGrant(ctx, ownerTestTenant, key.UUID, p.ID, "root")
	require.NoError(t, err)
	require.False(t, explicitAllowed(t, db, key.UUID))
}

func TestProfileProviderWriteFailureRollsBackEveryKeyAndProfileSelection(t *testing.T) {
	db, ctx, profile, key, original := ownerFixture(t)
	p := explicitPermissionFixture(t, db)
	other, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).Create(ctx, &gwmodel.IntegrationGatewayAPIKey{TenantUUID: ownerTestTenant, ProfileID: profile.ID, Name: "second", KeyHash: uuid.NewString(), Status: "active", PluginOwnerPolicy: EncodePluginOwners([]string{"com.example.second"})})
	require.NoError(t, err)
	// 比较数据库读回的原始快照，兼容 PostgreSQL JSONB 的键排序和空格规范化。
	before := map[uuid.UUID][]byte{}
	for _, k := range []*gwmodel.IntegrationGatewayAPIKey{key, other} {
		stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, k.UUID)
		require.NoError(t, err)
		before[k.UUID] = append([]byte(nil), stored.PluginOwnerPolicy...)
	}
	writes := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("profile-write-failure", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Model.(*gwmodel.IntegrationGatewayAPIKey); ok {
			writes++
			if writes == 2 {
				tx.AddError(errors.New("injected profile second key failure"))
			}
		}
	}))
	t.Cleanup(func() { db.Callback().Update().Remove("profile-write-failure") })
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := iamrepo.NewAPIKeyProfilePermissionRepository(tx).GrantByIDsTx(tx, profile.ID, []uint64{p.ID}); err != nil {
			return err
		}
		_, _, err := NewAPIKeyOwnerService(db).SyncProfileTx(ctx, tx, ownerTestTenant, profile.ID)
		return err
	})
	require.ErrorContains(t, err, "injected profile second key failure")
	require.Equal(t, 2, writes)
	for _, k := range []*gwmodel.IntegrationGatewayAPIKey{key, other} {
		require.False(t, explicitAllowed(t, db, k.UUID))
		stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, k.UUID)
		require.NoError(t, err)
		require.Equal(t, before[k.UUID], []byte(stored.PluginOwnerPolicy))
	}
	ids, err := iamrepo.NewAPIKeyProfilePermissionRepository(db).ListPermissionIDsOfProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, original, ids)
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.scrm"))
}
