package apikeypermissions

import (
	"context"
	"errors"
	"testing"
	"time"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBatchExplicitPreviewCommitPreservesBoundaries(t *testing.T) {
	db, ctx, profile, key, profileIDs := ownerFixture(t)
	p := explicitPermissionFixture(t, db)
	svc := NewAPIKeyOwnerService(db)
	makeKey := func(name string) *gwmodel.IntegrationGatewayAPIKey {
		k, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).Create(ctx, &gwmodel.IntegrationGatewayAPIKey{TenantUUID: ownerTestTenant, ProfileID: profile.ID, Name: name, KeyHash: uuid.NewString(), Status: "active", PluginOwnerPolicy: EncodePluginOwners([]string{"com.example." + name})})
		require.NoError(t, err)
		return k
	}
	second := makeKey("crm")
	untouched := makeKey("unselected")
	input := BatchExplicitGrantInput{KeyIDs: []uuid.UUID{second.UUID, key.UUID, key.UUID}, PermissionIDs: []uint64{p.ID, p.ID}, DryRun: true}
	preview, err := svc.BatchAppendExplicitGrants(ctx, ownerTestTenant, input, "root")
	require.NoError(t, err)
	require.False(t, preview.Committed)
	require.Equal(t, 2, preview.KeyCount)
	require.Equal(t, 2, preview.AddedCount)
	require.False(t, explicitAllowed(t, db, key.UUID))
	require.False(t, explicitAllowed(t, db, second.UUID))
	var count int64
	require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyAuditLog{}).Count(&count).Error)
	require.Zero(t, count)
	original, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, key.UUID)
	require.NoError(t, err)
	require.Empty(t, original.PluginOwnerPolicy, "preview must not materialize legacy policy")
	input.DryRun = false
	input.ExpectedPlanHash = preview.PlanHash
	saved, err := svc.BatchAppendExplicitGrants(ctx, ownerTestTenant, input, "root")
	require.NoError(t, err)
	require.True(t, saved.Committed)
	require.True(t, explicitAllowed(t, db, key.UUID))
	require.True(t, explicitAllowed(t, db, second.UUID))
	require.False(t, explicitAllowed(t, db, untouched.UUID))
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.scrm"))
	for _, k := range []*gwmodel.IntegrationGatewayAPIKey{key, second} {
		stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(ctx, k.UUID)
		require.NoError(t, err)
		require.Equal(t, k.KeyHash, stored.KeyHash)
		require.Equal(t, profile.ID, stored.ProfileID)
	}
	require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyAuditLog{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	ids, err := iamrepo.NewAPIKeyProfilePermissionRepository(db).ListPermissionIDsOfProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, profileIDs, ids)
	input.DryRun = true
	again, err := svc.BatchAppendExplicitGrants(ctx, ownerTestTenant, input, "root")
	require.NoError(t, err)
	require.Zero(t, again.AddedCount)
	input.DryRun = false
	input.ExpectedPlanHash = again.PlanHash
	_, err = svc.BatchAppendExplicitGrants(ctx, ownerTestTenant, input, "root")
	require.NoError(t, err)
	rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(ctx, key.UUID)
	require.NoError(t, err)
	require.Len(t, rows, 3, "retry preserves two original grants and one explicit grant")
}

func TestBatchExplicitValidationAndStalePreview(t *testing.T) {
	for _, reason := range []string{"missing-preview", "stale-owner", "stale-grants", "foreign-key", "inactive", "expired", "profile-disabled", "unpublished", "unregistered", "invalid-permission", "api-key-actor", "sts-actor", "member-actor"} {
		t.Run(reason, func(t *testing.T) {
			db, ctx, profile, key, ids := ownerFixture(t)
			p := explicitPermissionFixture(t, db)
			svc := NewAPIKeyOwnerService(db)
			input := BatchExplicitGrantInput{KeyIDs: []uuid.UUID{key.UUID}, PermissionIDs: []uint64{p.ID}, DryRun: true}
			preview, err := svc.BatchAppendExplicitGrants(ctx, ownerTestTenant, input, "root")
			require.NoError(t, err)
			input.DryRun = false
			input.ExpectedPlanHash = preview.PlanHash
			expected := ""
			switch reason {
			case "missing-preview":
				input.ExpectedPlanHash = ""
				expected = "API_KEY_BATCH_PREVIEW_REQUIRED"
			case "stale-owner":
				require.NoError(t, db.Model(key).Update("plugin_owner_policy", EncodePluginOwners([]string{"com.example.changed"})).Error)
				expected = "API_KEY_BATCH_PREVIEW_STALE"
			case "stale-grants":
				require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyPermission{}).Where("api_key_uuid = ?", key.UUID).Update("scope", "_scope.changed").Error)
				expected = "API_KEY_BATCH_PREVIEW_STALE"
			case "foreign-key":
				require.NoError(t, db.Model(key).Update("tenant_uuid", uuid.NewString()).Error)
				expected = "API_KEY_NOT_FOUND"
			case "inactive":
				require.NoError(t, db.Model(key).Update("status", "revoked").Error)
			case "expired":
				require.NoError(t, db.Model(key).Update("expires_at", time.Now().Add(-time.Minute)).Error)
			case "profile-disabled":
				require.NoError(t, db.Model(profile).Update("status", 0).Error)
			case "unpublished":
				require.NoError(t, db.Model(&capmodels.CapabilityRecord{}).Where("capability_id = ?", explicitTestCapability).Update("status", "draft").Error)
				expected = "CAPABILITY_UNKNOWN"
			case "unregistered":
				require.NoError(t, db.Model(&capmodels.CapabilityRegistration{}).Where("capability_id = ?", explicitTestCapability).Update("status", "disabled").Error)
				expected = "CAPABILITY_TENANT_NOT_REGISTERED"
			case "invalid-permission":
				input.PermissionIDs = append(input.PermissionIDs, ids[0])
			case "api-key-actor":
				ctx = reqctx.WithAuthenticatedAPIKeyHash(ctx, "test-actor")
			case "sts-actor":
				claims := &reqctx.CoreXClaims{TenantUUID: ownerTestTenant}
				claims.Issuer = "powerx-sts"
				ctx = reqctx.WithClaims(ctx, claims)
			case "member-actor":
				ctx = reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 3, TenantUUID: ownerTestTenant, Roles: []string{"role_user"}})
			}
			_, err = svc.BatchAppendExplicitGrants(ctx, ownerTestTenant, input, "root")
			require.Error(t, err)
			if expected != "" {
				require.Equal(t, expected, dto.CodeOf(err))
			}
			require.False(t, explicitAllowed(t, db, key.UUID))
			var count int64
			require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyAuditLog{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestBatchExplicitFailureAfterFirstWriteRollsBackEveryKeyAndAudit(t *testing.T) {
	db, ctx, profile, key, _ := ownerFixture(t)
	p := explicitPermissionFixture(t, db)
	svc := NewAPIKeyOwnerService(db)
	other, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).Create(ctx, &gwmodel.IntegrationGatewayAPIKey{TenantUUID: ownerTestTenant, ProfileID: profile.ID, Name: "second", KeyHash: uuid.NewString(), Status: "active"})
	require.NoError(t, err)
	input := BatchExplicitGrantInput{KeyIDs: []uuid.UUID{key.UUID, other.UUID}, PermissionIDs: []uint64{p.ID}, DryRun: true}
	preview, err := svc.BatchAppendExplicitGrants(ctx, ownerTestTenant, input, "root")
	require.NoError(t, err)
	writes := 0
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("batch-test-failure", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Model.(*gwmodel.IntegrationGatewayAPIKeyAuditLog); ok {
			writes++
			if writes == 2 {
				tx.AddError(errors.New("injected second key audit failure"))
			}
		}
	}))
	t.Cleanup(func() { db.Callback().Create().Remove("batch-test-failure") })
	input.DryRun = false
	input.ExpectedPlanHash = preview.PlanHash
	_, err = svc.BatchAppendExplicitGrants(ctx, ownerTestTenant, input, "root")
	require.ErrorContains(t, err, "injected second key")
	require.Equal(t, 2, writes)
	for _, k := range []*gwmodel.IntegrationGatewayAPIKey{key, other} {
		require.False(t, explicitAllowed(t, db, k.UUID))
		stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(context.Background(), k.UUID)
		require.NoError(t, err)
		require.Equal(t, k.PluginOwnerPolicy, stored.PluginOwnerPolicy)
	}
	require.True(t, ownerAllows(t, db, key.UUID, "com.powerx.plugins.scrm"))
	var count int64
	require.NoError(t, db.Model(&gwmodel.IntegrationGatewayAPIKeyAuditLog{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestBatchExplicitInputLimits(t *testing.T) {
	db, ctx, _, key, _ := ownerFixture(t)
	svc := NewAPIKeyOwnerService(db)
	for _, input := range []BatchExplicitGrantInput{{}, {KeyIDs: []uuid.UUID{uuid.Nil}, PermissionIDs: []uint64{1}}, {KeyIDs: []uuid.UUID{key.UUID}, PermissionIDs: []uint64{0}}, {KeyIDs: make([]uuid.UUID, 51), PermissionIDs: []uint64{1}}, {KeyIDs: []uuid.UUID{key.UUID}, PermissionIDs: make([]uint64, 129)}} {
		_, err := svc.BatchAppendExplicitGrants(ctx, ownerTestTenant, input, "root")
		require.Equal(t, "API_KEY_BATCH_GRANT_INVALID", dto.CodeOf(err))
	}
}
