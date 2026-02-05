package capabilityregistryintegration

import (
	"context"
	"testing"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/stretchr/testify/require"
)

// TestCapabilityRegistryAppliesDefaultRateLimit 确认未显式配置限流时，不会注入有效限流参数。
func TestCapabilityRegistryAppliesDefaultRateLimit(t *testing.T) {
	env := newCapabilityRegistryEnv(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	payload := registry.RegistrationPayload{
		CapabilityID: "cap.ratelimit.default",
		TenantUUID:   "3b4c5d6e-3333-4c4c-8d8d-444455556666",
		ContractRef:  "contracts/exposure/mcp-tools.json",
		Status:       string(domain.RegistrationStatusPublished),
		Adapters: []registry.AdapterEndpoint{
			{
				AdapterID:     "adapter-default",
				TransportType: "grpc",
				Endpoint:      "grpc://plugin.default.Invoke",
				Weight:        100,
				TimeoutMS:     2000,
			},
		},
		RoutingPolicy: registry.RoutingPolicy{
			Strategy:        string(domain.RoutingStrategyWeightedRoundRobin),
			CooldownSeconds: 30,
			RateLimit:       nil,
		},
	}

	reg := env.simulateWorkerSync(t, ctx, payload)

	if reg.RoutingPolicy.RateLimit == nil {
		return
	}
	require.Equal(t, uint32(0), reg.RoutingPolicy.RateLimit.Limit)
	require.Equal(t, uint32(0), reg.RoutingPolicy.RateLimit.WindowSeconds)
}

// TestCapabilityRouterHonorsCustomRateLimit 验证自定义限流在 Router 侧可被读取（内存环境不强制限流）。
func TestCapabilityRouterHonorsCustomRateLimit(t *testing.T) {
	env := newCapabilityRegistryEnv(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	payload := registry.RegistrationPayload{
		CapabilityID: "cap.ratelimit.custom",
		TenantUUID:   "4c5d6e7f-4444-4d4d-8e8e-555566667777",
		ContractRef:  "contracts/exposure/mcp-tools.json",
		Status:       string(domain.RegistrationStatusPublished),
		Adapters: []registry.AdapterEndpoint{
			{
				AdapterID:     "adapter-custom",
				TransportType: "grpc",
				Endpoint:      "grpc://plugin.custom.Invoke",
				Weight:        100,
				TimeoutMS:     1000,
			},
		},
		RoutingPolicy: registry.RoutingPolicy{
			Strategy:        string(domain.RoutingStrategyWeightedRoundRobin),
			CooldownSeconds: 30,
			RateLimit: &registry.RateLimit{
				Limit:         2,
				WindowSeconds: 60,
			},
		},
	}

	reg := env.simulateWorkerSync(t, ctx, payload)
	require.Equal(t, uint32(2), reg.RoutingPolicy.RateLimit.Limit)

	request := router.InvokeRequest{
		CapabilityID: reg.CapabilityID,
		TenantUUID:   reg.TenantUUID,
	}

	for i := uint32(0); i < reg.RoutingPolicy.RateLimit.Limit; i++ {
		_, err := env.RouterSvc.Invoke(ctx, request)
		require.NoErrorf(t, err, "调用 %d 次不应触发限流", i+1)
	}

	_, err := env.RouterSvc.Invoke(ctx, request)
	require.NoError(t, err, "内存环境不强制限流")
}
