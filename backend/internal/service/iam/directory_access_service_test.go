package iam

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	igwmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDirectoryAccessServiceAuthorizeMembersRead(t *testing.T) {
	db := newDirectoryAccessDB(t)
	tenantUUID := "1b5fbb8e-0a7a-4e95-a6c9-8c4354c2d123"
	pluginID := "com.powerx.plugin.directory-consumer"
	for _, capabilityID := range []string{directoryMembersReadCapabilityID, directoryCatalogReadCapabilityID, directoryAuthorizationCheckCapabilityID} {
		require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: capabilityID, PluginID: "com.powerx.core", PluginVersion: "v1", Title: "IAM directory", Status: "published"}).Error)
		require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: capabilityID, TenantUUID: tenantUUID, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}

	t.Run("allows an explicitly granted plugin service", func(t *testing.T) {
		grantCredential(t, db, tenantUUID, pluginID, []string{directoryMembersReadCapabilityID})
		actualTenant, err := NewDirectoryAccessService(db).AuthorizeMembersRead(directoryClaimsContext(tenantUUID, pluginID))
		require.NoError(t, err)
		require.Equal(t, tenantUUID, actualTenant)
	})

	t.Run("rejects same tenant plugin without capability grant", func(t *testing.T) {
		otherPluginID := "com.powerx.plugin.ungranted"
		grantCredential(t, db, tenantUUID, otherPluginID, nil)
		_, err := NewDirectoryAccessService(db).AuthorizeMembersRead(directoryClaimsContext(tenantUUID, otherPluginID))
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, dto.StatusCode(err))
		require.Equal(t, DirectoryReasonForbidden, dto.CodeOf(err))
	})

	t.Run("rejects a granted plugin when tenant capability registration is absent", func(t *testing.T) {
		unregisteredTenant := "6efc4ae7-770c-43a4-a6a2-09f0281ecf5c"
		unregisteredPlugin := "com.powerx.plugin.unregistered"
		grantCredential(t, db, unregisteredTenant, unregisteredPlugin, []string{directoryMembersReadCapabilityID})
		_, err := NewDirectoryAccessService(db).AuthorizeMembersRead(directoryClaimsContext(unregisteredTenant, unregisteredPlugin))
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, dto.StatusCode(err))
		require.Equal(t, DirectoryReasonForbidden, dto.CodeOf(err))
	})

	t.Run("rejects missing sts identity", func(t *testing.T) {
		_, err := NewDirectoryAccessService(db).AuthorizeMembersRead(context.Background())
		require.Error(t, err)
		require.Equal(t, http.StatusUnauthorized, dto.StatusCode(err))
		require.Equal(t, DirectoryReasonUnauthorized, dto.CodeOf(err))
	})

	t.Run("allows a gateway api key with the directory capability grant", func(t *testing.T) {
		keyHash := "api-key-hash-allowed"
		key := &igwmodels.IntegrationGatewayAPIKey{
			TenantUUID: tenantUUID, ProfileID: 1, Name: "test", KeyPrefix: "pxk_test", KeyHash: keyHash, Status: "active",
		}
		require.NoError(t, db.Create(key).Error)
		require.NoError(t, db.Create(&igwmodels.IntegrationGatewayAPIKeyPermission{
			APIKeyUUID: key.UUID, Scope: directoryMembersReadAPIKeyScope, Action: "read", ResourceType: "api", ResourcePattern: "*", Effect: "allow",
		}).Error)
		actualTenant, err := NewDirectoryAccessService(db).AuthorizeMembersReadAPIKey(directoryAPIKeyContext(tenantUUID), keyHash)
		require.NoError(t, err)
		require.Equal(t, tenantUUID, actualTenant)
	})

	t.Run("rejects a gateway api key without the directory capability grant", func(t *testing.T) {
		keyHash := "api-key-hash-ungranted"
		require.NoError(t, db.Create(&igwmodels.IntegrationGatewayAPIKey{
			TenantUUID: tenantUUID, ProfileID: 1, Name: "test-denied", KeyPrefix: "pxk_test", KeyHash: keyHash, Status: "active",
		}).Error)
		_, err := NewDirectoryAccessService(db).AuthorizeMembersReadAPIKey(directoryAPIKeyContext(tenantUUID), keyHash)
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, dto.StatusCode(err))
		require.Equal(t, DirectoryReasonForbidden, dto.CodeOf(err))
	})

	t.Run("requires distinct grants for catalog and authorization check", func(t *testing.T) {
		catalogPluginID := "com.powerx.plugin.directory-catalog"
		grantCredential(t, db, tenantUUID, catalogPluginID, []string{directoryCatalogReadCapabilityID, directoryAuthorizationCheckCapabilityID})
		actualTenant, err := NewDirectoryAccessService(db).AuthorizeDirectoryRead(directoryClaimsContext(tenantUUID, catalogPluginID))
		require.NoError(t, err)
		require.Equal(t, tenantUUID, actualTenant)
		actualTenant, err = NewDirectoryAccessService(db).AuthorizeAuthorizationCheck(directoryClaimsContext(tenantUUID, catalogPluginID))
		require.NoError(t, err)
		require.Equal(t, tenantUUID, actualTenant)

		keyHash := "api-key-hash-directory-catalog"
		key := &igwmodels.IntegrationGatewayAPIKey{TenantUUID: tenantUUID, ProfileID: 1, Name: "catalog", KeyPrefix: "pxk_test", KeyHash: keyHash, Status: "active"}
		require.NoError(t, db.Create(key).Error)
		require.NoError(t, db.Create(&igwmodels.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: directoryCatalogReadAPIKeyScope, Action: "read", ResourceType: "api", ResourcePattern: "*", Effect: "allow"}).Error)
		actualTenant, err = NewDirectoryAccessService(db).AuthorizeDirectoryReadAPIKey(directoryAPIKeyContext(tenantUUID), keyHash)
		require.NoError(t, err)
		require.Equal(t, tenantUUID, actualTenant)
		_, err = NewDirectoryAccessService(db).AuthorizeAuthorizationCheckAPIKey(directoryAPIKeyContext(tenantUUID), keyHash)
		require.Error(t, err)
		require.Equal(t, DirectoryReasonForbidden, dto.CodeOf(err))
	})
}

func newDirectoryAccessDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open("file:directory_access?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}, &dbsetting.PluginInstanceConfig{}, &igwmodels.IntegrationGatewayAPIKey{}, &igwmodels.IntegrationGatewayAPIKeyPermission{}))
	return db
}

func directoryAPIKeyContext(tenantUUID string) context.Context {
	claims := &reqctx.CoreXClaims{TenantUUID: tenantUUID, Platforms: []string{"api_key"}}
	return reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenantUUID), claims)
}

func grantCredential(t *testing.T, db *gorm.DB, tenantUUID, pluginID string, capabilities []string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"allowed_capabilities": capabilities})
	require.NoError(t, err)
	require.NoError(t, db.Where("tenant_uuid = ? AND plugin_id = ? AND key = ?", tenantUUID, pluginID, "auth.credentials").Delete(&dbsetting.PluginInstanceConfig{}).Error)
	require.NoError(t, db.Create(&dbsetting.PluginInstanceConfig{
		TenantUUID: tenantUUID,
		PluginID:   pluginID,
		Key:        "auth.credentials",
		ValueJSON:  datatypes.JSON(payload),
		Enabled:    true,
	}).Error)
}

func directoryClaimsContext(tenantUUID, pluginID string) context.Context {
	claims := &reqctx.CoreXClaims{
		TenantUUID: tenantUUID,
		PluginID:   pluginID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: []string{"powerx:api"},
		},
	}
	return reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenantUUID), claims)
}
