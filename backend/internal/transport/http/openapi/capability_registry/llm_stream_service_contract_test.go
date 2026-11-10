package capability_registry

import (
	"context"
	"encoding/json"
	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"path/filepath"
	"testing"
)

type contractLLMProvider struct{ calls int }

func (*contractLLMProvider) ResolveTenantEnv(context.Context, string) (string, bool, error) {
	return "dev", true, nil
}
func (p *contractLLMProvider) LLMStream(_ context.Context, _, _, _ string, _ []aisvc.ContentItem, _ map[string]any, delta func(string)) (string, error) {
	p.calls++
	delta("fixture-delta")
	return "fixture-delta", nil
}
func TestInvokeCapabilityStreamFormalLLMContractRequiresExactLiveGrant(t *testing.T) {
	provider := &contractLLMProvider{}
	f := newInvocationHTTPFixture(t, false, &aisvc.LLMStreamInvoker{Service: provider})
	t.Setenv("PLATFORM_CAPABILITIES_DIR", filepath.Join("..", "..", "..", "..", "..", "config", "platform_capabilities"))
	rows, err := apikeypermissions.BuildPlatformCapabilityPermissions()
	require.NoError(t, err)
	var permission *im.Permission
	for _, row := range rows {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if meta["capability_id"] == aisvc.LLMStreamCapability && meta["type"] == "capability_service" {
			copy := row
			permission = &copy
			break
		}
	}
	require.NotNil(t, permission)
	require.NoError(t, f.db.Create(permission).Error)
	// The old route grant alone cannot authorize the typed service capability.
	require.NoError(t, f.db.Where("api_key_uuid=?", f.key.UUID).Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: f.key.UUID, Scope: "_scope.ai.ai_llm_stream.create", Action: "create", ResourceType: "api", ResourcePattern: "POST:/api/v1/ai/llm/stream", Effect: "allow"}).Error)
	require.NoError(t, f.db.Where("resource=?", f.capID).Delete(&im.Permission{}).Error)
	bindings, _ := json.Marshal([]cm.ProtocolBinding{{Channel: "core_internal", Method: "INVOKE", Endpoint: aisvc.LLMStreamEndpoint, AuthType: "api_key_or_sts", ActorContext: "service_actor", ResourceScope: "tenant"}})
	require.NoError(t, f.db.Model(&cm.CapabilityRecord{}).Where("capability_id=?", f.capID).Update("protocols", datatypes.JSON(bindings)).Error)
	require.NoError(t, f.db.Model(&cm.AdapterEndpoint{}).Where("capability_id=?", f.capID).Updates(map[string]any{"transport_type": "core_internal", "endpoint": aisvc.LLMStreamEndpoint, "labels": datatypes.JSON(`{"method":"INVOKE"}`)}).Error)
	envelope := streamEnvelope(f)
	envelope["preferred_protocol"] = "core_internal"
	envelope["payload"].(map[string]any)["body"] = map[string]any{"model_key": "openai/fixture", "inputs": []any{map[string]any{"type": "text", "content": "fixture-input"}}}
	response := f.request(t, "/api/v1/tenant/invocations/stream?env=dev", envelope)
	require.Equal(t, 403, response.Code)
	require.Zero(t, provider.calls)
	require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: f.key.UUID, Scope: "_scope.ai.llm.stream", Action: "stream", ResourceType: "capability", ResourcePattern: "ai_llm_stream", Effect: "allow"}).Error)
	response = f.request(t, "/api/v1/tenant/invocations/stream?env=dev", envelope)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "event: delta")
	require.Contains(t, response.Body.String(), "event: done")
	require.Contains(t, response.Body.String(), `"ok":true`)
	require.Equal(t, 1, provider.calls)
	require.Zero(t, f.calls.Load())
	require.NoError(t, f.db.Where("api_key_uuid=? AND scope=?", f.key.UUID, "_scope.ai.llm.stream").Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	response = f.request(t, "/api/v1/tenant/invocations/stream?env=dev", envelope)
	require.Equal(t, 403, response.Code)
	require.Equal(t, 1, provider.calls)
}
