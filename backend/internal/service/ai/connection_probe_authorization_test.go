package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
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

type connectionModelVerifier struct{ calls atomic.Int32 }

func (v *connectionModelVerifier) VerifyModelKey(context.Context, string, string, string, string) error {
	v.calls.Add(1)
	return MediaError(400, "probe must not require model_key")
}

func TestConnectionServiceAPIKeySTSGrantsRevocationAndPublication(t *testing.T) {
	f := newMediaFixture(t)
	dir, err := filepath.Abs("../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv("PLATFORM_CAPABILITIES_DIR", dir)
	require.NoError(t, f.db.AutoMigrate(&audit.AuditEvent{}, &tenant.Tenant{}))
	row := tenant.Tenant{Key: "connection-test"}
	row.UUID = uuid.MustParse(mediaTestTenant)
	require.NoError(t, f.db.Create(&row).Error)
	ids := []string{ConnectionTestCapability, cap.GrantStatusCapabilityID}
	require.NoError(t, apikeypermissions.EnsureUpgradePermissionsForCapabilities(f.ctx, f.db, ids))
	require.NoError(t, gateway.NewBaseCapabilitySeeder(gateway.BaseCapabilitySeederOptions{CapabilityIDs: ids, PreserveInstalledState: true, RecordRepo: cr.NewCapabilityRecordRepository(f.db, nil), RegistryRepo: cr.NewCapabilityRegistryRepository(f.db), TenantRepo: tr.NewTenantRepository(f.db)}).Ensure(f.ctx))
	registry := cr.NewCapabilityRegistryRepository(f.db)
	reg, err := registry.GetLatest(f.ctx, nil, ConnectionTestCapability, mediaTestTenant)
	require.NoError(t, err)
	require.Len(t, reg.Adapters, 1, "Admin binding is published but never a service runtime adapter")
	require.Equal(t, "core_internal", reg.Adapters[0].TransportType)
	require.Equal(t, "docs/contracts/ai-connection-test.v1.openapi.yaml", reg.ContractRef)
	// 已登记的旧混合绑定也由正式 Seeder 升级；保留版本历史和授权。
	legacy := reg
	legacy.Version++
	legacy.RoutingPolicy.ID = uuid.Nil
	legacy.ContractRef = "docs/contracts/ai-settings-connection-test.md"
	legacy.Adapters = append(append([]cr.AdapterEndpoint(nil), reg.Adapters...), cr.AdapterEndpoint{AdapterID: "legacy-admin", TransportType: "http", Endpoint: "/api/v1/admin/agents/test/connection", ServiceRef: "/api/v1/admin/agents/test/connection", Labels: map[string]string{"method": "POST"}, IsActive: true})
	_, err = registry.Update(f.ctx, nil, legacy, reg.Version)
	require.NoError(t, err)
	require.NoError(t, gateway.NewBaseCapabilitySeeder(gateway.BaseCapabilitySeederOptions{CapabilityIDs: ids, PreserveInstalledState: true, RecordRepo: cr.NewCapabilityRecordRepository(f.db, nil), RegistryRepo: registry, TenantRepo: tr.NewTenantRepository(f.db)}).Ensure(f.ctx))
	reg, err = registry.GetLatest(f.ctx, nil, ConnectionTestCapability, mediaTestTenant)
	require.NoError(t, err)
	require.EqualValues(t, 3, reg.Version)
	require.Len(t, reg.Adapters, 1)
	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/tags", r.URL.Path)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	t.Cleanup(provider.Close)
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", ptr(mediaTestTenant), &am.AIProviderCredential{Name: "dev-ollama", Provider: "ollama", Data: datatypes.JSONMap{"base_url": provider.URL}}))
	verifier := &connectionModelVerifier{}
	invoker := &MediaInvoker{Service: f.s}
	service := cap.NewInvocationService(cap.InvocationServiceOptions{Catalog: cap.NewRegistryService(cap.RegistryServiceOptions{DB: f.db}), Router: caprouter.NewService(caprouter.ServiceOptions{DB: f.db}), CoreInvoker: invoker, ModelVerifier: verifier})
	key := gw.IntegrationGatewayAPIKey{TenantUUID: mediaTestTenant, ProfileID: 1, Name: "probe-only", KeyPrefix: "test", KeyHash: "connection-test-key", Status: "active"}
	require.NoError(t, f.db.Create(&key).Error)
	apiCtx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, Platforms: []string{"api_key"}}), key.KeyHash)
	credential := setting.PluginInstanceConfig{TenantUUID: mediaTestTenant, PluginID: "connection-test-plugin", Key: "auth.credentials", Enabled: true}
	setSTS := func(allowed []string) {
		raw, err := json.Marshal(map[string]any{"client_id": "connection-client", "allowed_capabilities": allowed})
		require.NoError(t, err)
		credential.ValueJSON = raw
		require.NoError(t, f.db.Save(&credential).Error)
	}
	setSTS([]string{cap.GrantStatusCapabilityID})
	stsCtx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, PluginID: credential.PluginID, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:connection-client", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	grant := func(scope, action, resource string) {
		require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: scope, Action: action, ResourceType: "api", ResourcePattern: resource, Effect: "allow"}).Error)
	}
	grant("_scope.capabilities.grant_status.read", "read", "grant-status")
	input := cap.InvocationInput{TenantUUID: mediaTestTenant, CapabilityID: ConnectionTestCapability, PreferredProtocol: "core_internal", Payload: map[string]any{"body": connectionTestBody("ollama")}}
	assertDenied := func(ctx context.Context) {
		_, err := service.Invoke(ctx, input)
		var denied *cap.DirectGrantError
		require.ErrorAs(t, err, &denied)
		require.Equal(t, 403, denied.Status)
	}
	for _, ctx := range []context.Context{apiCtx, stsCtx} {
		statuses, err := cap.NewGrantStatusService(f.db).CheckCurrentCredential(ctx, []string{ConnectionTestCapability})
		require.NoError(t, err)
		require.Equal(t, cap.GrantStatusNotGranted, statuses[0].Status)
		assertDenied(ctx)
	}
	grant("_scope.ai.connection.test", "test", "ai-connection-test")
	setSTS(ids)
	for _, ctx := range []context.Context{apiCtx, stsCtx} {
		statuses, err := cap.NewGrantStatusService(f.db).CheckCurrentCredential(ctx, []string{ConnectionTestCapability})
		require.NoError(t, err)
		require.Equal(t, cap.GrantStatusGranted, statuses[0].Status)
		result, err := service.Invoke(ctx, input)
		require.NoError(t, err)
		require.Equal(t, "completed", result.Status)
		require.Equal(t, "core_internal", result.ProtocolUsed)
		require.False(t, result.FallbackUsed)
		require.NotEmpty(t, result.TraceID)
		cross := reqctx.WithTenantUUID(ctx, uuid.NewString())
		_, err = service.Invoke(cross, input)
		require.Error(t, err)
	}
	require.EqualValues(t, 2, providerCalls.Load())
	require.Zero(t, verifier.calls.Load(), "probe without model must pass the actual generic invocation path")
	// Published and registered state is evaluated live, even with an existing context/token.
	for _, model := range []any{&cm.CapabilityRecord{}, &cm.CapabilityRegistration{}} {
		require.NoError(t, f.db.Model(model).Where("capability_id = ?", ConnectionTestCapability).Update("status", "disabled").Error)
		assertDenied(apiCtx)
		assertDenied(stsCtx)
		require.NoError(t, f.db.Model(model).Where("capability_id = ?", ConnectionTestCapability).Update("status", "published").Error)
	}
	require.NoError(t, f.db.Where("api_key_uuid = ? AND scope = ?", key.UUID, "_scope.ai.connection.test").Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	setSTS([]string{cap.GrantStatusCapabilityID})
	assertDenied(apiCtx)
	assertDenied(stsCtx)
	require.EqualValues(t, 2, providerCalls.Load())
	require.Zero(t, f.generations.Load())
}
