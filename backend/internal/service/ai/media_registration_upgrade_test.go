package ai

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/agent/toolstore"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	caprouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	gateway "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	audit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	cr "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	tr "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// 复用升级前同一凭据走真实 Registry/Router/MediaInvoker，供应商采用本地受控响应。
func TestMediaLegacyRegistrationUpgradeOriginalCredentials(t *testing.T) {
	f := newMediaFixture(t)
	dir, err := filepath.Abs("../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv("PLATFORM_CAPABILITIES_DIR", dir)
	require.NoError(t, f.db.AutoMigrate(&audit.AuditEvent{}, &tenant.Tenant{}))
	row := tenant.Tenant{Key: "original-media-tenant"}
	row.UUID = uuid.MustParse(mediaTestTenant)
	require.NoError(t, f.db.Create(&row).Error)
	specs := []struct{ id, endpoint, method, scope, action, resource string }{
		{"com.corex.ai.catalog.providers.read", "/api/v1/admin/agents/providers", "GET", "_scope.ai.catalog.providers.read", "read", "ai-catalog-providers"},
		{MediaCatalogCapability, "/api/v1/admin/agents/models", "GET", "_scope.ai.catalog.models.read", "read", "ai-catalog-models"},
		{"com.corex.ai.vlm.invoke", "/api/v1/ai/vlm/invoke", "POST", "_scope.ai.vlm.invoke", "invoke", "ai_vlm"},
		{"com.corex.ai.image.invoke", "/api/v1/ai/image/invoke", "POST", "_scope.ai.image.invoke", "invoke", "ai_image"},
		{"com.corex.ai.video.invoke", "/api/v1/ai/video/invoke", "POST", "_scope.ai.video.invoke", "invoke", "ai_video"},
		{"com.corex.ai.tts.invoke", "/api/v1/ai/tts/invoke", "POST", "_scope.ai.tts.invoke", "invoke", "ai_tts"},
		{MediaTaskReadCapability, "/api/v1/ai/media/tasks/{task_uuid}", "GET", "_scope.ai.media.tasks.read", "read", "ai_media_tasks"},
		{MediaTaskCancelCapability, "/api/v1/ai/media/tasks/{task_uuid}/cancel", "POST", "_scope.ai.media.tasks.cancel", "cancel", "ai_media_tasks"},
	}
	ids := []string{}
	for _, spec := range specs {
		ids = append(ids, spec.id)
		labels, err := json.Marshal(map[string]string{"method": spec.method})
		require.NoError(t, err)
		protocols, err := json.Marshal([]cm.ProtocolBinding{{Channel: "rest", Endpoint: spec.endpoint, Method: spec.method}})
		require.NoError(t, err)
		require.NoError(t, f.db.Create(&cm.CapabilityRecord{CapabilityID: spec.id, PluginID: gateway.PlatformPluginID, PluginVersion: "legacy", Title: spec.id, Status: "published", CapabilitiesHash: strings.Repeat("a", 64), ProtocolHash: "legacy", Protocols: protocols}).Error)
		policy := cm.RoutingPolicy{Strategy: "weighted"}
		require.NoError(t, f.db.Create(&policy).Error)
		ref := "specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml"
		if strings.Contains(spec.id, ".catalog.") {
			ref = "backend/internal/transport/http/admin/agent/api.go"
		}
		reg := cm.CapabilityRegistration{CapabilityID: spec.id, TenantUUID: mediaTestTenant, ContractRef: ref, Status: "published", Version: 1, RoutingPolicyID: policy.UUID}
		require.NoError(t, f.db.Create(&reg).Error)
		require.NoError(t, f.db.Create(&cm.AdapterEndpoint{RegistrationID: reg.ID, CapabilityID: spec.id, TenantUUID: mediaTestTenant, AdapterID: spec.id + "-legacy", TransportType: "http", Endpoint: spec.endpoint, ServiceRef: spec.endpoint, IsActive: true, Weight: 100, TimeoutMS: 5000, Labels: labels}).Error)
	}
	key := gw.IntegrationGatewayAPIKey{TenantUUID: mediaTestTenant, ProfileID: 1, Name: "original-media-plugin", KeyPrefix: "test", KeyHash: "original-plugin-key", Status: "active"}
	require.NoError(t, f.db.Create(&key).Error)
	for _, spec := range specs {
		resourceType := "capability"
		if strings.Contains(spec.id, ".catalog.") {
			resourceType = "api"
		}
		require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: spec.scope, Action: spec.action, ResourceType: resourceType, ResourcePattern: spec.resource, Effect: "allow"}).Error)
	}
	require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.capabilities.grant_status.read", Action: "read", ResourceType: "api", ResourcePattern: "grant-status", Effect: "allow"}).Error)
	allowed, err := json.Marshal(map[string]any{"client_id": "original-client", "allowed_capabilities": append(append([]string{}, ids...), cap.GrantStatusCapabilityID)})
	require.NoError(t, err)
	require.NoError(t, f.db.Create(&setting.PluginInstanceConfig{TenantUUID: mediaTestTenant, PluginID: "original-media-plugin", Key: "auth.credentials", Enabled: true, ValueJSON: allowed}).Error)
	apiCtx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, Platforms: []string{"api_key"}}), key.KeyHash)
	stsCtx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, PluginID: "original-media-plugin", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:original-client", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	require.NoError(t, apikeypermissions.EnsureUpgradePermissions(f.ctx, f.db))
	lock := toolstore.NewVersionLockStore(toolstore.VersionLockStoreOptions{})
	for _, id := range ids {
		require.NoError(t, lock.Bind(f.ctx, mediaTestTenant, id, strings.Repeat("a", 64)))
	}
	invokeSvc := cap.NewInvocationService(cap.InvocationServiceOptions{Catalog: cap.NewRegistryService(cap.RegistryServiceOptions{DB: f.db}), Router: caprouter.NewService(caprouter.ServiceOptions{DB: f.db}), CoreInvoker: &MediaInvoker{Service: f.s}, VersionLock: lock})
	_, err = cap.NewVersionUpgradeService(f.db, lock).Confirm(adminCatalogContext(), cap.VersionUpgradeInput{TenantUUID: mediaTestTenant, CapabilityID: MediaCatalogCapability, CapabilitiesHash: strings.Repeat("a", 64), Reason: "before migration"})
	require.ErrorIs(t, err, cap.ErrVersionUpgradeBindingMismatch)
	require.NoError(t, gateway.NewBaseCapabilitySeeder(gateway.BaseCapabilitySeederOptions{RecordRepo: cr.NewCapabilityRecordRepository(f.db, nil), RegistryRepo: cr.NewCapabilityRegistryRepository(f.db), TenantRepo: tr.NewTenantRepository(f.db), PreserveInstalledState: true}).Ensure(f.ctx))
	_, err = invokeSvc.Invoke(stsCtx, cap.InvocationInput{TenantUUID: mediaTestTenant, CapabilityID: MediaCatalogCapability, PreferredProtocol: "core_internal", Payload: map[string]any{"body": map[string]any{"operation": "list_models", "capability": catalog.ImageGeneration}}})
	require.ErrorIs(t, err, cap.ErrManualUpgradeRequired)
	for _, id := range ids {
		var record cm.CapabilityRecord
		require.NoError(t, f.db.Where("capability_id = ?", id).First(&record).Error)
		_, err = cap.NewVersionUpgradeService(f.db, lock).Confirm(adminCatalogContext(), cap.VersionUpgradeInput{TenantUUID: mediaTestTenant, CapabilityID: id, CapabilitiesHash: record.CapabilitiesHash, Reason: "approve installed media v2 catalog"})
		require.NoError(t, err)
	}
	invoke := func(ctx context.Context, id string, body map[string]any) cap.InvocationResult {
		got, err := invokeSvc.Invoke(ctx, cap.InvocationInput{TenantUUID: mediaTestTenant, CapabilityID: id, PreferredProtocol: "core_internal", Payload: map[string]any{"body": body}})
		require.NoError(t, err)
		require.Equal(t, "core_internal", got.ProtocolUsed)
		require.False(t, got.FallbackUsed)
		require.NotEmpty(t, got.TraceID)
		return got
	}
	for _, ctx := range []context.Context{apiCtx, stsCtx} {
		statuses, err := cap.NewGrantStatusService(f.db).CheckCurrentCredential(ctx, ids)
		require.NoError(t, err)
		for _, status := range statuses {
			require.Equal(t, cap.GrantStatusGranted, status.Status)
		}
		for _, ability := range []string{catalog.VisionUnderstanding, catalog.ImageGeneration, catalog.VideoGeneration, catalog.SpeechSynthesis} {
			providers := invoke(ctx, specs[0].id, map[string]any{"operation": "list_providers", "capability": ability}).Result["result"].(map[string]any)
			require.EqualValues(t, 2, providers["contract_version"])
			require.NotEmpty(t, providers["providers"])
			models := invoke(ctx, MediaCatalogCapability, map[string]any{"operation": "list_models", "capability": ability}).Result["result"].(map[string]any)
			require.EqualValues(t, 2, models["contract_version"])
			require.NotEmpty(t, models["items"])
		}
		for _, tc := range []struct{ ability, model string }{{catalog.VisionUnderstanding, "qwen-cn/qwen-vl-max"}, {catalog.ImageGeneration, "qwen-cn/qwen-image-2.0-pro"}, {catalog.VideoGeneration, videoInput().ModelKey}, {catalog.SpeechSynthesis, "qwen-cn/qwen3-tts-flash"}} {
			body := map[string]any{"operation": "invoke", "contract_version": 2, "model_key": tc.model, "inputs": []any{map[string]any{"type": "text", "content": "controlled validation"}}, "idempotency_key": uuid.NewString()}
			if tc.ability == catalog.VisionUnderstanding {
				body["params"] = map[string]any{"temperature": 0}
			}
			result := invoke(ctx, InvokeCapability(tc.ability), body)
			if tc.ability == catalog.VisionUnderstanding {
				continue
			}
			task := result.Result["result"].(*MediaTaskView)
			invoke(ctx, MediaTaskReadCapability, map[string]any{"operation": "get", "task_uuid": task.TaskUUID})
			invoke(ctx, MediaTaskCancelCapability, map[string]any{"operation": "cancel", "task_uuid": task.TaskUUID})
		}
	}
	// GET/查询不会触发供应商生成；取消的是尚未领取的 Core 任务。
	// 同一原有服务身份通过正式 core_internal 目录与 VLM 操作消费新 OpenAI 视觉声明。
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", ptr(mediaTestTenant), &agentmodel.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "test-only", "base_url": f.provider.URL + "/v1"}}))
	require.NoError(t, f.db.Create(&agentmodel.AIModelProfile{Env: "dev", TenantUUID: ptr(mediaTestTenant), Modality: "vlm", Provider: "openai", Model: "gpt-6-luna", Defaults: datatypes.JSONMap{"reasoning_effort": "none"}}).Error)
	for _, identity := range []context.Context{apiCtx, stsCtx} {
		models := invoke(identity, MediaCatalogCapability, map[string]any{"operation": "list_models", "provider": "openai", "capability": catalog.VisionUnderstanding}).Result["result"].(map[string]any)
		items := models["items"].([]MediaModelItem)
		require.Len(t, items, 1)
		require.Equal(t, "openai/gpt-6-luna", items[0].ModelKey)
		require.True(t, items[0].Configured)
		require.True(t, items[0].Available)
		analysis := invoke(identity, "com.corex.ai.vlm.invoke", map[string]any{"operation": "invoke", "contract_version": 2, "model_key": "openai/gpt-6-luna", "inputs": []any{map[string]any{"type": "text", "content": "describe"}, map[string]any{"type": "image_url", "url": "https://example.invalid/picture.png"}}, "params": map[string]any{"temperature": 0}}).Result["result"].(map[string]any)
		require.Equal(t, "analysis", analysis["type"])
		require.NotEmpty(t, analysis["text"])
		require.NotEmpty(t, analysis["trace_id"])
	}
	require.Zero(t, f.generations.Load())
	require.NoError(t, f.db.Where("api_key_uuid = ? AND scope = ?", key.UUID, specs[1].scope).Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	_, err = invokeSvc.Invoke(apiCtx, cap.InvocationInput{TenantUUID: mediaTestTenant, CapabilityID: MediaCatalogCapability, PreferredProtocol: "core_internal", Payload: map[string]any{"body": map[string]any{"operation": "list_models"}}})
	require.Error(t, err)
	cross := reqctx.WithTenantUUID(apiCtx, uuid.NewString())
	_, err = invokeSvc.Invoke(cross, cap.InvocationInput{TenantUUID: mediaTestTenant, CapabilityID: MediaCatalogCapability, PreferredProtocol: "core_internal", Payload: map[string]any{"body": map[string]any{"operation": "list_models"}}})
	require.Error(t, err)
}

func adminCatalogContext() context.Context {
	return reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{UserID: 1, TenantUUID: mediaTestTenant, IsRoot: true})
}
