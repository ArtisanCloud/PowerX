package runtime_identity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	settings "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTypedIdentityAuthorization(t *testing.T) {
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &gwmodel.IntegrationGatewayAPIKey{}, &gwmodel.IntegrationGatewayAPIKeyPermission{}, &settings.PluginInstanceConfig{}))
	tenant, plugin := uuid.NewString(), "com.powerx.plugins.base"
	require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: CapabilityID, PluginID: "com.powerx.core", PluginVersion: "v1", Title: "Identity", Status: "published"}).Error)
	require.NoError(t, db.Create(&capmodel.CapabilityRegistration{CapabilityID: CapabilityID, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	key := gwmodel.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: 1, Name: "Host", KeyHash: "identity-key", KeyPrefix: "pxk", Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	invoker := NewInvoker(db, CoreInfo{})
	invoker.service = testService(identityManager{version: "1.2.3", runtimeOK: true, state: supervisor.ProcRunning})
	api := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}}), key.KeyHash)
	input := cap.CoreCapabilityInvokeInput{CapabilityID: CapabilityID, TenantUUID: tenant, Method: "INVOKE", Endpoint: "core://runtime/identity", Body: map[string]any{"operation": "get", "plugin_id": plugin}}
	call := func(ctx context.Context, status int) {
		t.Helper()
		result, err := invoker.InvokeCoreCapability(ctx, input)
		if status == 200 {
			require.NoError(t, err)
			require.Equal(t, plugin, result["item"].(*Identity).PluginID)
		} else {
			require.Equal(t, status, dto.StatusCode(err))
		}
	}
	call(api, 403)
	permission := gwmodel.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: APIKeyScope, Action: "read", ResourceType: "capability", ResourcePattern: "runtime_identity_read", Effect: "allow"}
	require.NoError(t, db.Create(&permission).Error)
	call(api, 403) // blank plugin binding is forbidden
	require.NoError(t, db.Model(&permission).Update("plugin_id", plugin).Error)
	call(api, 200)
	input.Body["plugin_id"] = "another.plugin"
	call(api, 403)
	input.Body["plugin_id"] = plugin
	input.TenantUUID = uuid.NewString()
	call(api, 403)
	input.TenantUUID = tenant
	input.Payload = map[string]any{"headers": map[string]any{}}
	call(api, 400)
	input.Payload = nil
	input.Body["tenant_uuid"] = tenant
	call(api, 400)
	delete(input.Body, "tenant_uuid")
	input.Endpoint = "http://attacker.invalid"
	call(api, 400)
	input.Endpoint = "core://runtime/identity"
	require.NoError(t, db.Delete(&permission).Error)
	call(api, 403)
	sts := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: plugin, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}})
	call(sts, 403)
	raw, _ := json.Marshal(map[string]any{"allowed_capabilities": []string{CapabilityID}})
	credential := settings.PluginInstanceConfig{TenantUUID: tenant, PluginID: plugin, Key: "auth.credentials", ValueJSON: datatypes.JSON(raw), Enabled: true}
	require.NoError(t, db.Create(&credential).Error)
	call(sts, 200)
	input.Body["plugin_id"] = "another.plugin"
	call(sts, 403)
	input.Body["plugin_id"] = plugin
	require.NoError(t, db.Model(&credential).Update("enabled", false).Error)
	call(sts, 403)
	require.NoError(t, db.Model(&credential).Update("enabled", true).Error)
	require.NoError(t, db.Model(&capmodel.CapabilityRegistration{}).Where("tenant_uuid = ?", tenant).Update("status", "disabled").Error)
	call(sts, 403)
}
