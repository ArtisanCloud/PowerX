package knowledge_space

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gwmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	settingmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func TestHostContractAccessRequiresRegistrationAndCredentialGrant(t *testing.T) {
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open("file:knowledge_host_access_"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}, &settingmodels.PluginInstanceConfig{}, &gwmodels.IntegrationGatewayAPIKey{}, &gwmodels.IntegrationGatewayAPIKeyPermission{}))
	tenantUUID, pluginID := uuid.NewString(), "com.powerx.plugin.knowledge-consumer"
	for _, capabilityID := range []string{KnowledgeDirectoryReadCapabilityID, KnowledgeSearchReadCapabilityID, KnowledgeDocumentManageCapabilityID} {
		require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: capabilityID, PluginID: "com.powerx.core", PluginVersion: "v1", Title: "Knowledge", Status: "published"}).Error)
		require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: capabilityID, TenantUUID: tenantUUID, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
	access := NewHostContractAccess(db)

	payload, err := json.Marshal(map[string]any{"allowed_capabilities": []string{KnowledgeSearchReadCapabilityID}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&settingmodels.PluginInstanceConfig{TenantUUID: tenantUUID, PluginID: pluginID, Key: "auth.credentials", ValueJSON: datatypes.JSON(payload), Enabled: true}).Error)
	actualTenant, err := access.AuthorizeSearchRead(hostSTSContext(tenantUUID, pluginID), "")
	require.NoError(t, err)
	require.Equal(t, tenantUUID, actualTenant)
	_, err = access.AuthorizeDocumentManage(hostSTSContext(tenantUUID, pluginID), "")
	require.Equal(t, http.StatusForbidden, dto.StatusCode(err))
	require.Equal(t, KnowledgeReasonForbidden, dto.CodeOf(err))

	keyHash := "knowledge-api-key"
	key := &gwmodels.IntegrationGatewayAPIKey{TenantUUID: tenantUUID, ProfileID: 1, Name: "knowledge", KeyPrefix: "pxk", KeyHash: keyHash, Status: "active"}
	require.NoError(t, db.Create(key).Error)
	require.NoError(t, db.Create(&gwmodels.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: knowledgeDirectoryAPIKeyScope, Action: "read", ResourceType: "api", ResourcePattern: "*", Effect: "allow"}).Error)
	actualTenant, err = access.AuthorizeDirectoryRead(hostAPIKeyContext(tenantUUID), keyHash)
	require.NoError(t, err)
	require.Equal(t, tenantUUID, actualTenant)
	_, err = access.AuthorizeSearchRead(hostAPIKeyContext(tenantUUID), keyHash)
	require.Equal(t, KnowledgeReasonForbidden, dto.CodeOf(err))
}

func hostSTSContext(tenantUUID, pluginID string) context.Context {
	claims := &reqctx.CoreXClaims{TenantUUID: tenantUUID, PluginID: pluginID, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: []string{"powerx:api"}}}
	return reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenantUUID), claims)
}
func hostAPIKeyContext(tenantUUID string) context.Context {
	claims := &reqctx.CoreXClaims{TenantUUID: tenantUUID, Platforms: []string{"api_key"}}
	return reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenantUUID), claims)
}
