package knowledge_space

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gwmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	settings "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestProvisioningTypedInvokerAPIKeyAndSTS(t *testing.T) {
	svc, db, tenant, department := hostProvisioningFixture(t)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}, &settings.PluginInstanceConfig{}, &gwmodels.IntegrationGatewayAPIKey{}, &gwmodels.IntegrationGatewayAPIKeyPermission{}))
	caps := []string{KnowledgeCatalogReadCapabilityID, KnowledgeSpaceCreateCapabilityID}
	for _, id := range caps {
		require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: id, PluginID: "com.powerx.core", PluginVersion: "v1", Title: "Knowledge", Status: "published"}).Error)
		require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: id, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
	plugin := "com.powerx.plugins.base"
	raw, _ := json.Marshal(map[string]any{"allowed_capabilities": caps})
	require.NoError(t, db.Create(&settings.PluginInstanceConfig{TenantUUID: tenant, PluginID: plugin, Key: "auth.credentials", ValueJSON: datatypes.JSON(raw), Enabled: true}).Error)
	key := gwmodels.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: 1, Name: "Host", KeyPrefix: "pxk", KeyHash: "typed-host-key", Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	for _, p := range []gwmodels.IntegrationGatewayAPIKeyPermission{
		{APIKeyUUID: key.UUID, Scope: "_scope.knowledge.catalog.read", Action: "read", ResourceType: "api", ResourcePattern: "catalog", Effect: "allow"},
		{APIKeyUUID: key.UUID, Scope: "_scope.knowledge.space.create", Action: "create", ResourceType: "api", ResourcePattern: "space", Effect: "allow"},
	} {
		require.NoError(t, db.Create(&p).Error)
	}
	invoker := NewProvisioningCapabilityInvoker(db, func() *Service { return svc })
	for _, mode := range []struct {
		name string
		ctx  context.Context
	}{
		{"api_key", reqctx.WithAuthenticatedAPIKeyHash(hostAPIKeyContext(tenant), key.KeyHash)},
		{"sts", hostSTSContext(tenant, plugin)},
	} {
		input := cap.CoreCapabilityInvokeInput{CapabilityID: KnowledgeCatalogReadCapabilityID, TenantUUID: tenant, Method: "INVOKE", Endpoint: "core://knowledge/catalog", Body: map[string]any{"operation": "catalog"}}
		response, err := invoker.InvokeCoreCapability(mode.ctx, input)
		require.NoError(t, err)
		require.NotNil(t, response["catalog"])
		input.CapabilityID = KnowledgeSpaceCreateCapabilityID
		input.Endpoint = "core://knowledge/spaces"
		input.Body = map[string]any{"operation": "create", "name": "created-by-" + mode.name, "department_uuid": department, "strategy_key": "A_simple"}
		response, err = invoker.InvokeCoreCapability(mode.ctx, input)
		require.NoError(t, err)
		item := response["item"].(*HostCreatedSpace)
		items, err := NewHostContractService(db).ListSpaces(mode.ctx, tenant)
		require.NoError(t, err)
		found := false
		for _, space := range items {
			if space.SpaceUUID == item.SpaceUUID {
				found = true
			}
		}
		require.True(t, found)
		input.Body["tenant_uuid"] = tenant
		_, err = invoker.InvokeCoreCapability(mode.ctx, input)
		require.Equal(t, http.StatusBadRequest, dto.StatusCode(err))
		delete(input.Body, "tenant_uuid")
		input.Payload = map[string]any{"headers": map[string]any{"Authorization": "override"}}
		_, err = invoker.InvokeCoreCapability(mode.ctx, input)
		require.Equal(t, http.StatusBadRequest, dto.StatusCode(err))
		input.Payload = nil
		input.Endpoint = "http://attacker.invalid"
		_, err = invoker.InvokeCoreCapability(mode.ctx, input)
		require.Equal(t, http.StatusBadRequest, dto.StatusCode(err))
	}
}
