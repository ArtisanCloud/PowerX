package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestImage2ServiceKeyCatalogDefaultsAfterScopedRepair(t *testing.T) {
	f := newMediaFixture(t)
	for _, id := range []string{MediaCatalogCapability, InvokeCapability(catalog.ImageGeneration)} {
		require.NoError(t, f.db.Create(&cm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "test", Title: id, CapabilitiesHash: id, ProtocolHash: id, Status: "published"}).Error)
		policy := cm.RoutingPolicy{Strategy: "weighted_round_robin"}
		require.NoError(t, f.db.Create(&policy).Error)
		require.NoError(t, f.db.Create(&cm.CapabilityRegistration{CapabilityID: id, TenantUUID: mediaTestTenant, ContractRef: "test", Status: "published", Version: 1, RoutingPolicyID: policy.UUID}).Error)
	}
	key := gw.IntegrationGatewayAPIKey{TenantUUID: mediaTestTenant, ProfileID: 1, Name: "image-default-test", KeyPrefix: "test", KeyHash: "image-default-test-hash", Status: "active"}
	require.NoError(t, f.db.Create(&key).Error)
	for _, spec := range []struct{ id, scope, resourceType, resource string }{
		{MediaCatalogCapability, "_scope.ai.catalog.models.read", "api", "ai-catalog-models"},
		{InvokeCapability(catalog.ImageGeneration), "_scope.ai.image.invoke", "capability", "ai_image"},
	} {
		action := "invoke"
		if spec.id == MediaCatalogCapability {
			action = "read"
		}
		metadata, _ := json.Marshal(map[string]any{"capability_id": spec.id, "api_key_explicit": true, "api_key": map[string]any{"scope": spec.scope, "action": action, "resource_type": spec.resourceType, "resource_pattern": spec.resource, "effect": "allow"}})
		require.NoError(t, f.db.Create(&iam.Permission{Module: "ai", Resource: spec.resource, Action: action, Effect: "allow", Status: iam.PermissionStatusActive, Source: "platform_capability", AllowAPIKey: true, Meta: metadata}).Error)
		require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: spec.scope, Action: action, ResourceType: spec.resourceType, ResourcePattern: spec.resource, Effect: "allow"}).Error)
	}
	ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, Platforms: []string{"api_key"}}), key.KeyHash)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-image-2"}]}`))
	}))
	defer provider.Close()
	tenant := mediaTestTenant
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", &tenant, &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "fixture-only", "base_url": provider.URL + "/v1"}}))
	require.NoError(t, f.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "image", Provider: "openai", Model: "gpt-image-2", Defaults: datatypes.JSONMap{"quality": "standard", "size": "1024x1024", "format": "png"}}).Error)
	read := func() MediaModelItem {
		result, err := (&MediaInvoker{Service: f.s}).InvokeCoreCapability(ctx, cap.CoreCapabilityInvokeInput{CapabilityID: MediaCatalogCapability, TenantUUID: tenant, Method: "INVOKE", Endpoint: MediaCatalogEndpoint, Body: map[string]any{"operation": "list_models", "provider": "openai", "capability": catalog.ImageGeneration}})
		require.NoError(t, err)
		raw, err := json.Marshal(result["result"])
		require.NoError(t, err)
		var decoded struct {
			Items []MediaModelItem `json:"items"`
		}
		require.NoError(t, json.Unmarshal(raw, &decoded))
		require.Len(t, decoded.Items, 1)
		return decoded.Items[0]
	}
	before := read()
	require.True(t, before.Configured)
	require.False(t, before.Available)
	require.Equal(t, "AI_MODEL_DEFAULTS_INVALID", before.ReasonCode)
	plan, err := f.s.ai.settings.PreviewImageQualityRepair(ctx, tenant)
	require.NoError(t, err)
	changed, err := f.s.ai.settings.ApplyImageQualityRepair(ctx, *plan)
	require.NoError(t, err)
	require.True(t, changed)
	after := read()
	require.True(t, after.Configured)
	require.True(t, after.Available, after.ReasonCode)
	op := after.Operations[catalog.ImageGeneration]
	require.True(t, op.Available)
	require.Empty(t, op.ReasonCode)
	require.Equal(t, "auto", op.Defaults["quality"])
	require.NotEmpty(t, op.ParameterSchema)
	require.Zero(t, f.generations.Load(), "目录读取和修复不触发生图")
}
