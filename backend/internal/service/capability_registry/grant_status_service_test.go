package capability_registry

import (
	"context"
	"fmt"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const grantStatusTestTenant = "11111111-1111-1111-1111-111111111111"

func TestGrantStatusServiceSTSReturnsOrderedEffectiveStatuses(t *testing.T) {
	db := newGrantStatusTestDB(t)
	seedGrantStatusCapability(t, db, GrantStatusCapabilityID, true)
	seedGrantStatusCapability(t, db, "com.powerx.plugins.scrm.leads.read", true)
	seedGrantStatusCapability(t, db, "com.powerx.plugins.scrm.leads.write", true)
	seedGrantStatusCapability(t, db, "com.powerx.plugins.scrm.contacts.read", false)
	require.NoError(t, db.Create(&settingmodel.PluginInstanceConfig{
		TenantUUID: grantStatusTestTenant,
		PluginID:   "com.powerx.plugins.scrm",
		Key:        "auth.credentials",
		Enabled:    true,
		ValueJSON:  datatypes.JSON([]byte(`{"allowed_capabilities":["com.corex.capabilities.grant_status.read","com.powerx.plugins.scrm.leads.read"]}`)),
	}).Error)
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), grantStatusTestTenant), &reqctx.CoreXClaims{
		TenantUUID:       grantStatusTestTenant,
		PluginID:         "com.powerx.plugins.scrm",
		RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}},
	})
	items, err := NewGrantStatusService(db).CheckCurrentCredential(ctx, []string{
		"com.powerx.plugins.scrm.leads.write", "com.powerx.plugins.scrm.contacts.read", "com.powerx.plugins.scrm.leads.read", "com.powerx.plugins.scrm.missing.read",
	})
	require.NoError(t, err)
	require.Equal(t, []GrantStatusItem{
		{CapabilityID: "com.powerx.plugins.scrm.leads.write", Status: GrantStatusNotGranted, ReasonCode: GrantStatusReasonNotGranted},
		{CapabilityID: "com.powerx.plugins.scrm.contacts.read", Status: GrantStatusUnknown, ReasonCode: GrantStatusReasonTenantNotRegistered},
		{CapabilityID: "com.powerx.plugins.scrm.leads.read", Status: GrantStatusGranted, ReasonCode: GrantStatusReasonGranted},
		{CapabilityID: "com.powerx.plugins.scrm.missing.read", Status: GrantStatusUnknown, ReasonCode: GrantStatusReasonUnknown},
	}, items)
}

func TestGrantStatusGatewayKeyUsesActualKeyPermission(t *testing.T) {
	db := newGrantStatusTestDB(t)
	require.NoError(t, db.AutoMigrate(&gwmodel.IntegrationGatewayAPIKey{}, &gwmodel.IntegrationGatewayAPIKeyPermission{}))
	capabilityID := "com.corex.knowledge.directory.read"
	seedGrantStatusCapability(t, db, GrantStatusCapabilityID, true)
	seedGrantStatusCapability(t, db, capabilityID, true)
	for _, permission := range []iammodel.Permission{
		{Module: "capability_registry", Resource: "grant-status", Action: "read", Effect: "allow", Status: iammodel.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON([]byte(`{"capability_id":"com.corex.capabilities.grant_status.read","api_key_explicit":true,"api_key":{"scope":"_scope.capabilities.grant_status.read","action":"read","resource_type":"api","resource_pattern":"grant-status","effect":"allow"}}`))},
		{Module: "knowledge", Resource: "directory", Action: "read", Effect: "allow", Status: iammodel.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON([]byte(`{"capability_id":"com.corex.knowledge.directory.read","api_key_explicit":true,"api_key":{"scope":"_scope.knowledge.directory.read","action":"read","resource_type":"api","resource_pattern":"directory","effect":"allow"}}`))},
	} {
		require.NoError(t, db.Create(&permission).Error)
	}
	key := gwmodel.IntegrationGatewayAPIKey{TenantUUID: grantStatusTestTenant, ProfileID: 1, Name: "test", KeyPrefix: "test", KeyHash: "test-key-hash", Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	for _, p := range []gwmodel.IntegrationGatewayAPIKeyPermission{
		{APIKeyUUID: key.UUID, Scope: "_scope.capabilities.grant_status.read", Action: "read", ResourceType: "api", ResourcePattern: "grant-status", Effect: "allow"},
		{APIKeyUUID: key.UUID, Scope: "_scope.knowledge.directory.read", Action: "read", ResourceType: "api", ResourcePattern: "directory", Effect: "allow"},
	} {
		require.NoError(t, db.Create(&p).Error)
	}
	ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), grantStatusTestTenant), &reqctx.CoreXClaims{TenantUUID: grantStatusTestTenant, MemberID: 1, Platforms: []string{"api_key"}}), key.KeyHash)
	service := NewGrantStatusService(db)
	items, err := service.CheckCurrentCredential(ctx, []string{capabilityID})
	require.NoError(t, err)
	require.Equal(t, GrantStatusGranted, items[0].Status)
	otherKey := gwmodel.IntegrationGatewayAPIKey{TenantUUID: grantStatusTestTenant, ProfileID: key.ProfileID, Name: "test-other", KeyPrefix: "test", KeyHash: "test-other-key-hash", Status: "active"}
	require.NoError(t, db.Create(&otherKey).Error)
	require.NoError(t, db.Create(&gwmodel.IntegrationGatewayAPIKeyPermission{APIKeyUUID: otherKey.UUID, Scope: "_scope.capabilities.grant_status.read", Action: "read", ResourceType: "api", ResourcePattern: "grant-status", Effect: "allow"}).Error)
	otherItems, err := service.CheckCurrentCredential(reqctx.WithAuthenticatedAPIKeyHash(ctx, otherKey.KeyHash), []string{capabilityID})
	require.NoError(t, err)
	require.Equal(t, GrantStatusNotGranted, otherItems[0].Status)
	require.NoError(t, db.Where("api_key_uuid = ? AND scope = ?", key.UUID, "_scope.knowledge.directory.read").Delete(&gwmodel.IntegrationGatewayAPIKeyPermission{}).Error)
	items, err = service.CheckCurrentCredential(ctx, []string{capabilityID})
	require.NoError(t, err)
	require.Equal(t, GrantStatusNotGranted, items[0].Status)
	require.NoError(t, db.Create(&capmodels.CapabilityRegistration{TenantUUID: grantStatusTestTenant, CapabilityID: capabilityID, Status: "disabled", Version: 2, ContractRef: "test", RoutingPolicyID: uuid.New()}).Error)
	items, err = service.CheckCurrentCredential(ctx, []string{capabilityID})
	require.NoError(t, err)
	require.Equal(t, GrantStatusUnknown, items[0].Status)
	require.Equal(t, GrantStatusReasonTenantNotRegistered, items[0].ReasonCode)
	require.NoError(t, db.Model(&key).Update("status", "revoked").Error)
	_, err = service.CheckCurrentCredential(ctx, []string{capabilityID})
	require.ErrorIs(t, err, ErrGrantStatusUnauthorized)
}

func TestGrantStatusServiceRequiresGrantStatusCapabilityForSTSAndAPIKey(t *testing.T) {
	t.Run("sts allowed capabilities", func(t *testing.T) {
		db := newGrantStatusTestDB(t)
		seedGrantStatusCapability(t, db, GrantStatusCapabilityID, true)
		seedGrantStatusCapability(t, db, "com.powerx.plugins.scrm.leads.read", true)
		require.NoError(t, db.Create(&settingmodel.PluginInstanceConfig{
			TenantUUID: grantStatusTestTenant, PluginID: "com.powerx.plugins.scrm", Key: "auth.credentials", Enabled: true,
			ValueJSON: datatypes.JSON([]byte(`{"allowed_capabilities":["com.powerx.plugins.scrm.leads.read"]}`)),
		}).Error)
		ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), grantStatusTestTenant), &reqctx.CoreXClaims{
			TenantUUID: grantStatusTestTenant, PluginID: "com.powerx.plugins.scrm",
			RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}},
		})
		_, err := NewGrantStatusService(db).CheckCurrentCredential(ctx, []string{"com.powerx.plugins.scrm.leads.read"})
		require.ErrorIs(t, err, ErrGrantStatusForbidden)
	})

	t.Run("api key exact scope", func(t *testing.T) {
		db := newGrantStatusTestDB(t)
		seedGrantStatusCapability(t, db, GrantStatusCapabilityID, true)
		seedGrantStatusCapability(t, db, "com.powerx.plugins.scrm.leads.read", true)
		profile := iammodel.APIKeyProfile{TenantUUID: grantStatusTestTenant, Key: "test", Name: "test", Status: 1}
		require.NoError(t, db.Create(&profile).Error)
		permission := iammodel.Permission{Module: "capability_registry", Resource: "grant-status", Action: "read", Effect: "allow", Status: iammodel.PermissionStatusActive,
			Meta: datatypes.JSON([]byte(`{"capability_id":"com.corex.capabilities.grant_status.read"}`))}
		require.NoError(t, db.Create(&permission).Error)
		require.NoError(t, db.Create(&iammodel.APIKeyProfilePermission{ProfileID: profile.ID, PermissionID: permission.ID}).Error)
		ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), grantStatusTestTenant), &reqctx.CoreXClaims{TenantUUID: grantStatusTestTenant, MemberID: profile.ID, Platforms: []string{"api_key"}})
		_, err := NewGrantStatusService(db).CheckCurrentCredential(ctx, []string{"com.powerx.plugins.scrm.leads.read"})
		require.ErrorIs(t, err, ErrGrantStatusUnauthorized)
	})
}

func TestGrantStatusServiceRequiresPublishedAndRegisteredGrantStatusCapability(t *testing.T) {
	for _, tt := range []struct {
		name           string
		seedCapability bool
		registered     bool
	}{
		{name: "not published", seedCapability: false},
		{name: "not registered", seedCapability: true, registered: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := newGrantStatusTestDB(t)
			if tt.seedCapability {
				seedGrantStatusCapability(t, db, GrantStatusCapabilityID, tt.registered)
			}
			seedGrantStatusCapability(t, db, "com.powerx.plugins.scrm.leads.read", true)
			require.NoError(t, db.Create(&settingmodel.PluginInstanceConfig{
				TenantUUID: grantStatusTestTenant, PluginID: "com.powerx.plugins.scrm", Key: "auth.credentials", Enabled: true,
				ValueJSON: datatypes.JSON([]byte(`{"allowed_capabilities":["com.corex.capabilities.grant_status.read","com.powerx.plugins.scrm.leads.read"]}`)),
			}).Error)
			ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), grantStatusTestTenant), &reqctx.CoreXClaims{
				TenantUUID: grantStatusTestTenant, PluginID: "com.powerx.plugins.scrm",
				RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}},
			})
			_, err := NewGrantStatusService(db).CheckCurrentCredential(ctx, []string{"com.powerx.plugins.scrm.leads.read"})
			require.ErrorIs(t, err, ErrGrantStatusForbidden)
		})
	}
}

func TestGrantStatusServiceRejectsDuplicateAndNonServiceCredentials(t *testing.T) {
	db := newGrantStatusTestDB(t)
	service := NewGrantStatusService(db)
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), grantStatusTestTenant), &reqctx.CoreXClaims{TenantUUID: grantStatusTestTenant})
	_, err := service.CheckCurrentCredential(ctx, []string{"com.powerx.plugins.scrm.leads.read", "com.powerx.plugins.scrm.leads.read"})
	require.ErrorIs(t, err, ErrGrantStatusInvalid)
	_, err = service.CheckCurrentCredential(ctx, []string{"com.powerx.plugins.scrm.leads.read"})
	require.ErrorIs(t, err, ErrGrantStatusForbidden)
	for _, claims := range []*reqctx.CoreXClaims{
		{TenantUUID: grantStatusTestTenant, PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "not-powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}},
		{TenantUUID: grantStatusTestTenant, PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"not-powerx-api"}}},
	} {
		_, err = service.CheckCurrentCredential(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), grantStatusTestTenant), claims), []string{"com.powerx.plugins.scrm.leads.read"})
		require.ErrorIs(t, err, ErrGrantStatusForbidden)
	}
}

func seedGrantStatusCapability(t *testing.T, db *gorm.DB, capabilityID string, registered bool) {
	t.Helper()
	require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: capabilityID, PluginID: "com.powerx.plugins.scrm", PluginVersion: "1.0.0", Title: capabilityID, CapabilitiesHash: capabilityID, ProtocolHash: capabilityID, Status: "published"}).Error)
	if registered {
		require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: capabilityID, TenantUUID: grantStatusTestTenant, ContractRef: "test", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
}

func TestGrantStatusLatestRegistrationOverridesHistoricalPublishedVersion(t *testing.T) {
	db := newGrantStatusTestDB(t)
	capabilityID := "com.powerx.plugins.scrm.leads.read"
	seedGrantStatusCapability(t, db, capabilityID, true)
	require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: capabilityID, TenantUUID: grantStatusTestTenant, ContractRef: "test", Status: "disabled", Version: 2, RoutingPolicyID: uuid.New()}).Error)
	state, err := NewGrantStatusService(db).publishedAndRegistered(context.Background(), grantStatusTestTenant, []string{capabilityID})
	require.NoError(t, err)
	require.True(t, state.published[capabilityID])
	require.False(t, state.registered[capabilityID])
	items := buildGrantStatusItems([]string{capabilityID}, state, map[string]bool{capabilityID: true})
	require.Equal(t, GrantStatusUnknown, items[0].Status)
	require.Equal(t, GrantStatusReasonTenantNotRegistered, items[0].ReasonCode)
}

func newGrantStatusTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{},
		&settingmodel.PluginInstanceConfig{}, &iammodel.APIKeyProfile{}, &iammodel.Permission{}, &iammodel.APIKeyProfilePermission{},
	))
	return db
}
