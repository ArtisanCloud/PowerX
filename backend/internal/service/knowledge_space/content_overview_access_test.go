package knowledge_space

import (
	"encoding/json"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gwmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	settingmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestContentOverviewRequiresExactLiveGrantAndRegistration(t *testing.T) {
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}, &settingmodels.PluginInstanceConfig{}, &gwmodels.IntegrationGatewayAPIKey{}, &gwmodels.IntegrationGatewayAPIKeyPermission{}))
	tenant, plugin := uuid.NewString(), "com.powerx.plugins.content-consumer"
	capID := KnowledgeContentOverviewCapabilityID
	require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: capID, PluginID: "com.powerx.core", PluginVersion: "v1", Title: "Content", Status: "published"}).Error)
	registration := capmodels.CapabilityRegistration{CapabilityID: capID, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}
	require.NoError(t, db.Create(&registration).Error)
	access := NewHostContractAccess(db)
	ctx := hostSTSContext(tenant, plugin)
	_, err = access.AuthorizeContentOverviewRead(ctx, "")
	require.Equal(t, 403, dto.StatusCode(err))
	payload, _ := json.Marshal(map[string]any{"allowed_capabilities": []string{capID}})
	creds := settingmodels.PluginInstanceConfig{TenantUUID: tenant, PluginID: plugin, Key: "auth.credentials", ValueJSON: datatypes.JSON(payload), Enabled: true}
	require.NoError(t, db.Create(&creds).Error)
	_, err = access.AuthorizeContentOverviewRead(ctx, "")
	require.NoError(t, err)
	require.NoError(t, db.Model(&creds).Update("enabled", false).Error)
	_, err = access.AuthorizeContentOverviewRead(ctx, "")
	require.Equal(t, 403, dto.StatusCode(err))
	key := gwmodels.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: 1, Name: "content", KeyPrefix: "pxk", KeyHash: "content-test", Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	apiCtx := hostAPIKeyContext(tenant)
	// Existing directory access must not authorize source text enumeration.
	require.NoError(t, db.Create(&gwmodels.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: knowledgeDirectoryAPIKeyScope, Action: "read", ResourceType: "api", ResourcePattern: "*", Effect: "allow"}).Error)
	_, err = access.AuthorizeContentOverviewRead(apiCtx, key.KeyHash)
	require.Equal(t, 403, dto.StatusCode(err))
	grant := gwmodels.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.knowledge.content_overview.service_read", Action: "read", ResourceType: "api", ResourcePattern: "content_overview", Effect: "allow"}
	require.NoError(t, db.Create(&grant).Error)
	_, err = access.AuthorizeContentOverviewRead(apiCtx, key.KeyHash)
	require.NoError(t, err)
	_, err = access.AuthorizeContentOverviewRead(hostAPIKeyContext(uuid.NewString()), key.KeyHash)
	require.Equal(t, 403, dto.StatusCode(err))
	require.NoError(t, db.Model(&registration).Update("status", "withdrawn").Error)
	_, err = access.AuthorizeContentOverviewRead(apiCtx, key.KeyHash)
	require.Equal(t, 403, dto.StatusCode(err))
	require.NoError(t, db.Model(&registration).Update("status", "published").Error)
	require.NoError(t, db.Delete(&grant).Error)
	_, err = access.AuthorizeContentOverviewRead(apiCtx, key.KeyHash)
	require.Equal(t, 403, dto.StatusCode(err))
}
