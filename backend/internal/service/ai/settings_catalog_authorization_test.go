package ai

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

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
)

func TestSettingsCatalogAPIKeySTSGrantsRevocationAndTenantIsolation(t *testing.T) {
	f := newMediaFixture(t)
	dir, err := filepath.Abs("../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv("PLATFORM_CAPABILITIES_DIR", dir)
	require.NoError(t, f.db.AutoMigrate(&audit.AuditEvent{}, &tenant.Tenant{}))
	row := tenant.Tenant{Key: "settings-catalog-test"}
	row.UUID = uuid.MustParse(mediaTestTenant)
	require.NoError(t, f.db.Create(&row).Error)
	ids := []string{settingsProvidersCapability, MediaCatalogCapability, cap.GrantStatusCapabilityID}
	require.NoError(t, apikeypermissions.EnsureUpgradePermissionsForCapabilities(f.ctx, f.db, ids))
	require.NoError(t, gateway.NewBaseCapabilitySeeder(gateway.BaseCapabilitySeederOptions{CapabilityIDs: ids, PreserveInstalledState: true, RecordRepo: cr.NewCapabilityRecordRepository(f.db, nil), RegistryRepo: cr.NewCapabilityRegistryRepository(f.db), TenantRepo: tr.NewTenantRepository(f.db)}).Ensure(f.ctx))
	invoker := &MediaInvoker{Service: f.s}
	service := cap.NewInvocationService(cap.InvocationServiceOptions{Catalog: cap.NewRegistryService(cap.RegistryServiceOptions{DB: f.db}), Router: caprouter.NewService(caprouter.ServiceOptions{DB: f.db}), CoreInvoker: invoker})
	key := gw.IntegrationGatewayAPIKey{TenantUUID: mediaTestTenant, ProfileID: 1, Name: "settings-reader", KeyPrefix: "test", KeyHash: "settings-catalog-key-hash", Status: "active"}
	require.NoError(t, f.db.Create(&key).Error)
	apiCtx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, Platforms: []string{"api_key"}}), key.KeyHash)
	credential := setting.PluginInstanceConfig{TenantUUID: mediaTestTenant, PluginID: "settings-test-plugin", Key: "auth.credentials", Enabled: true}
	setSTS := func(allowed []string) {
		raw, err := json.Marshal(map[string]any{"client_id": "settings-test-client", "allowed_capabilities": allowed})
		require.NoError(t, err)
		credential.ValueJSON = raw
		require.NoError(t, f.db.Save(&credential).Error)
	}
	setSTS(nil)
	stsCtx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, PluginID: credential.PluginID, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:settings-test-client", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	bodies := map[string]map[string]any{
		settingsProvidersCapability: {"operation": settingsProvidersOperation, "modality": "llm"},
		MediaCatalogCapability:      {"operation": settingsModelsOperation, "modality": "llm", "provider": "qwen-cn", "app": ""},
	}
	assertDenied := func(ctx context.Context, id string) {
		_, err := invoker.InvokeCoreCapability(ctx, settingsCatalogInput(id, bodies[id]))
		var denied *cap.DirectGrantError
		require.ErrorAs(t, err, &denied)
		require.Equal(t, 403, denied.Status)
	}
	for _, ctx := range []context.Context{apiCtx, stsCtx} {
		for id := range bodies {
			assertDenied(ctx, id)
		}
	}
	grant := func(scope, resource string) {
		require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: scope, Action: "read", ResourceType: "api", ResourcePattern: resource, Effect: "allow"}).Error)
	}
	grant("_scope.ai.catalog.providers.read", "ai-catalog-providers")
	setSTS([]string{settingsProvidersCapability})
	for _, ctx := range []context.Context{apiCtx, stsCtx} {
		_, err := service.Invoke(ctx, cap.InvocationInput{TenantUUID: mediaTestTenant, CapabilityID: settingsProvidersCapability, PreferredProtocol: "core_internal", Payload: map[string]any{"body": bodies[settingsProvidersCapability]}})
		require.NoError(t, err)
		assertDenied(ctx, MediaCatalogCapability)
	}
	grant("_scope.ai.catalog.models.read", "ai-catalog-models")
	grant("_scope.capabilities.grant_status.read", "grant-status")
	setSTS(ids)
	for _, ctx := range []context.Context{apiCtx, stsCtx} {
		statuses, err := cap.NewGrantStatusService(f.db).CheckCurrentCredential(ctx, ids[:2])
		require.NoError(t, err)
		for _, status := range statuses {
			require.Equal(t, cap.GrantStatusGranted, status.Status)
		}
		for id, body := range bodies {
			result, err := service.Invoke(ctx, cap.InvocationInput{TenantUUID: mediaTestTenant, CapabilityID: id, PreferredProtocol: "core_internal", Payload: map[string]any{"body": body}})
			require.NoError(t, err)
			require.Equal(t, "core_internal", result.ProtocolUsed)
			require.False(t, result.FallbackUsed)
			require.NotEmpty(t, result.TraceID)
			// 同一张已发出的凭据，非法字段不能变成自由 Admin 代理。
			bad := map[string]any{}
			for key, value := range body {
				bad[key] = value
			}
			bad["base_url"] = "http://caller-selected.invalid"
			_, err = invoker.InvokeCoreCapability(ctx, settingsCatalogInput(id, bad))
			requireMediaCode(t, err, "AI_OPERATION_INVALID")
		}
		cross := reqctx.WithTenantUUID(ctx, uuid.NewString())
		_, err = invoker.InvokeCoreCapability(cross, settingsCatalogInput(settingsProvidersCapability, bodies[settingsProvidersCapability]))
		requireMediaCode(t, err, "AI_FORBIDDEN")
	}
	// API Key scope 撤销、STS allowed_capabilities 撤销立即作用于原上下文。
	require.NoError(t, f.db.Where("api_key_uuid = ? AND scope = ?", key.UUID, "_scope.ai.catalog.models.read").Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	setSTS([]string{settingsProvidersCapability, cap.GrantStatusCapabilityID})
	assertDenied(apiCtx, MediaCatalogCapability)
	assertDenied(stsCtx, MediaCatalogCapability)
	// 服务读取还必须受当前 publication 与 registration 状态约束。
	require.NoError(t, f.db.Model(&cm.CapabilityRecord{}).Where("capability_id = ?", settingsProvidersCapability).Update("status", "draft").Error)
	assertDenied(apiCtx, settingsProvidersCapability)
	assertDenied(stsCtx, settingsProvidersCapability)
	require.NoError(t, f.db.Model(&cm.CapabilityRecord{}).Where("capability_id = ?", settingsProvidersCapability).Update("status", "published").Error)
	require.NoError(t, f.db.Model(&cm.CapabilityRegistration{}).Where("capability_id = ? AND tenant_uuid = ?", settingsProvidersCapability, mediaTestTenant).Update("status", "disabled").Error)
	assertDenied(apiCtx, settingsProvidersCapability)
	assertDenied(stsCtx, settingsProvidersCapability)
	require.Zero(t, f.generations.Load())
}
