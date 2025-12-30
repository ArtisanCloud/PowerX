package capabilityregistryintegration

import (
	"context"
	"testing"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/stretchr/testify/require"
)

const (
	defaultRateLimitRequests uint32 = 60
	defaultRateLimitWindow   uint32 = 60
)

// TestCapabilityRegistryAppliesDefaultRateLimit 确认未显式配置限流时，Registry 会注入平台默认令牌桶。
func TestCapabilityRegistryAppliesDefaultRateLimit(t *testing.T) {
	env := newCapabilityRegistryEnv(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	payload := registry.RegistrationPayload{
		CapabilityID: "cap.ratelimit.default",
		TenantUUID:   "tenant-rate-default",
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

	require.NotNil(t, reg.RoutingPolicy.RateLimit, "默认限流策略应当被填充")
	require.Equal(t, defaultRateLimitRequests, reg.RoutingPolicy.RateLimit.Limit)
	require.Equal(t, defaultRateLimitWindow, reg.RoutingPolicy.RateLimit.WindowSeconds)
}

// TestCapabilityRouterHonorsCustomRateLimit 验证自定义限流在 Router 侧会生效，超过阈值后返回限流错误。
func TestCapabilityRouterHonorsCustomRateLimit(t *testing.T) {
	env := newCapabilityRegistryEnv(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	payload := registry.RegistrationPayload{
		CapabilityID: "cap.ratelimit.custom",
		TenantUUID:   "tenant-rate-custom",
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
			CooldownSeconds: 15,
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
	require.Error(t, err, "超过自定义限流阈值后应返回错误")
	require.ErrorContains(t, err, "rate limit")
}
