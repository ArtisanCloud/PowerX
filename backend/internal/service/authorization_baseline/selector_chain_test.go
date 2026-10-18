package authorization_baseline

import (
	"context"
	"encoding/json"
	"testing"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// 使用真实策略生成、Selector、InvocationService、Router 和基线服务，避免
// 直接调用 typed invoker 的测试漏掉网关生成的路由元数据。
func TestBaselineThroughSelectorChain(t *testing.T) {
	f := newFixture(t, false)
	codes := []string{"production.sample_track:read", "production.bulk_order:read", "operations.business_dashboard.cost:read"}
	for _, p := range []im.Permission{
		{Module: "production", Resource: "sample_track", Action: "read"},
		{Module: "production", Resource: "bulk_order", Action: "read"},
		{Module: "operations.business_dashboard", Resource: "cost", Action: "read"},
	} {
		p.Effect, p.Status, p.Source = "allow", im.PermissionStatusActive, "plugin:"+f.plugin
		p.Meta = datatypes.JSON(`{"authorization_baseline":true}`)
		require.NoError(t, f.db.Create(&p).Error)
	}
	f.initialize(t, codes...)
	selector := baselineSelector(t, f)
	request := func() cap.CapabilityInvokeRequest {
		return cap.CapabilityInvokeRequest{
			CapabilityID: CapabilityID, TenantUUID: f.tenant, PreferredProtocol: "core_internal",
			Payload: map[string]interface{}{"body": map[string]interface{}{"operation": Operation, "request": f.query(codes...)}},
		}
	}

	for _, scenario := range []struct {
		name       string
		capability string
		intent     string
		scope      string
	}{
		{name: "default scope", capability: CapabilityID},
		{name: "explicit scope", capability: CapabilityID, scope: "iam.authorization_baseline.read"},
		{name: "intent mapping", intent: "iam.authorization_baseline.read", scope: "iam.authorization_baseline.read"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			req := request()
			req.CapabilityID, req.Intent, req.ToolScope = scenario.capability, scenario.intent, scenario.scope
			response, err := selector.Invoke(f.svc, req)
			require.NoError(t, err)
			require.Equal(t, "completed", response.Status)
			require.Equal(t, "core_internal", response.ProtocolUsed)
			result, ok := response.Result["result"].(Result)
			require.True(t, ok)
			require.Equal(t, f.tenant, result.TenantUUID)
			require.Len(t, result.Items, len(codes))
			for _, item := range result.Items {
				require.Equal(t, "decided", item.Status)
				require.NotNil(t, item.Allowed)
			}
			require.Empty(t, req.Context, "不得修改调用方的 context")
		})
	}
	// 追踪尚未初始化是可查询的业务状态，不应再被路由 context 误报为参数错误。
	require.NoError(t, f.db.Model(&im.AuthorizationTracking{}).Where("tenant_uuid=? AND permission_code=?", f.tenant, codes[2]).Update("status", "uninitialized").Error)
	response, err := selector.Invoke(f.svc, request())
	require.NoError(t, err)
	result := response.Result["result"].(Result)
	require.Equal(t, "IAM_BASELINE_UNAVAILABLE", result.Items[2].ErrorCode)
	require.Nil(t, result.Items[2].Allowed)

	for _, callerContext := range []map[string]interface{}{
		{"tool_scope": "iam.authorization_baseline.read"},
		{"headers": map[string]interface{}{"Authorization": "caller-controlled"}},
		{"tenant_uuid": uuid.NewString()},
		{"routing": map[string]interface{}{"tool_scope": "iam.authorization_baseline.read"}},
	} {
		req := request()
		req.Context = callerContext
		_, err := selector.Invoke(f.svc, req)
		require.Equal(t, "IAM_BASELINE_INVALID_ARGUMENT", dto.CodeOf(err), "调用方 context 仍必须被拒绝")
	}
	for _, key := range []string{"headers", "endpoint", "method", "query", "tenant_uuid"} {
		req := request()
		req.Payload[key] = "caller-controlled"
		_, err := selector.Invoke(f.svc, req)
		require.Equal(t, "IAM_BASELINE_INVALID_ARGUMENT", dto.CodeOf(err), key)
	}

	// 路由修复不跳过真实服务授权：同一凭据撤销 grant 后立即拒绝。
	require.NoError(t, f.db.Where("api_key_uuid=?", f.key.UUID).Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	_, err = selector.Invoke(f.svc, request())
	var denied *cap.DirectGrantError
	require.ErrorAs(t, err, &denied)
	require.Equal(t, 403, denied.Status)
}

func baselineSelector(t *testing.T, f fixture) *cap.Selector {
	t.Helper()
	f.db.DisableForeignKeyConstraintWhenMigrating = true
	require.NoError(t, f.db.AutoMigrate(&cm.RoutingPolicy{}, &cm.FallbackPlan{}, &cm.AdapterEndpoint{}))
	policy := cm.RoutingPolicy{Strategy: "weighted_round_robin", CooldownSeconds: 30}
	require.NoError(t, f.db.Create(&policy).Error)
	var registration cm.CapabilityRegistration
	require.NoError(t, f.db.Where("tenant_uuid=? AND capability_id=?", f.tenant, CapabilityID).Take(&registration).Error)
	require.NoError(t, f.db.Model(&registration).Update("routing_policy_id", policy.UUID).Error)
	require.NoError(t, f.db.Create(&cm.AdapterEndpoint{
		RegistrationID: registration.ID, CapabilityID: CapabilityID, TenantUUID: f.tenant,
		AdapterID: "baseline-core-internal", TransportType: "core_internal", Endpoint: Endpoint,
		Weight: 100, TimeoutMS: 2000, IsActive: true, Labels: datatypes.JSON(`{"method":"INVOKE"}`),
	}).Error)
	protocols, err := json.Marshal([]map[string]interface{}{{"channel": "core_internal", "endpoint": Endpoint, "method": "INVOKE"}})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&cm.CapabilityRecord{}).Where("capability_id=?", CapabilityID).Updates(map[string]interface{}{
		"policy": datatypes.JSON(`{"prefer":"core_internal"}`), "protocols": protocols,
		"intents": datatypes.JSON(`["iam.authorization_baseline.read"]`), "tool_scope": datatypes.JSON(`["iam.authorization_baseline.read"]`),
	}).Error)
	catalog := cap.NewRegistryService(cap.RegistryServiceOptions{DB: f.db})
	invocation := cap.NewInvocationService(cap.InvocationServiceOptions{
		Catalog: catalog, Router: router.NewService(router.ServiceOptions{DB: f.db}),
		CoreInvoker: cap.NewCoreCapabilityMux(&Invoker{Service: f.s}),
	})
	generator := cap.NewPolicyGenerator(cap.PolicyGeneratorOptions{DB: f.db})
	return cap.NewSelector(cap.SelectorOptions{
		Store: cap.SnapshotProviderFunc(func(ctx context.Context, tenant string, grants []string) (cap.SelectorPolicySnapshot, error) {
			return generator.GenerateSnapshot(ctx, cap.GeneratePolicyInput{TenantUUID: tenant, ToolGrantIDs: grants, SkipCache: true})
		}),
		Invoker: invocation, Authorizer: cap.NewAuthorizationService(cap.AuthorizationOptions{Catalog: catalog}),
	})
}
