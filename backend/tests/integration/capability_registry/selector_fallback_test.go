package capabilityregistryintegration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
)

func TestCapabilityRouterFallbackToStaticResponse(t *testing.T) {
	env := newCapabilityRegistryEnv(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	fallbackEvents := make(chan event_bus.Event, 1)
	unsub := env.Bus.Subscribe(router.EventRouterFallback, func(evt event_bus.Event) error {
		select {
		case fallbackEvents <- evt:
		default:
		}
		return nil
	})
	t.Cleanup(unsub)

	payload := registry.RegistrationPayload{
		CapabilityID: "cap.fallback.demo",
		TenantUUID:   "tenant-fallback-001",
		ContractRef:  "contracts/exposure/mcp-tools.json",
		Status:       string(domain.RegistrationStatusPublished),
		Adapters: []registry.AdapterEndpoint{
			{
				AdapterID:     "adapter-mcp-primary",
				TransportType: "mcp",
				Endpoint:      "mcp://plugin.demo/tools/list",
				ServiceRef:    "plugin-demo",
				Weight:        100,
				TimeoutMS:     2000,
			},
		},
		RoutingPolicy: registry.RoutingPolicy{
			Strategy:        string(domain.RoutingStrategyWeightedRoundRobin),
			CooldownSeconds: 30,
		},
		FallbackPlan: &registry.FallbackPlan{
			StaticResponse: &registry.StaticResponse{
				Payload: map[string]interface{}{
					"status":  "fallback",
					"message": "use static response",
				},
				TTLSeconds: 30,
			},
		},
	}

	reg := env.simulateWorkerSync(t, ctx, payload)
	require.Equal(t, uint64(1), reg.Version)

	require.NoError(t, env.RouterSvc.ReportHealth(ctx, router.ReportHealthInput{
		CapabilityID: reg.CapabilityID,
		TenantUUID:   reg.TenantUUID,
		AdapterID:    "adapter-mcp-primary",
		Status:       "unhealthy",
		Reason:       "mcp channel outage",
	}))

	result, err := env.RouterSvc.Invoke(ctx, router.InvokeRequest{
		CapabilityID: reg.CapabilityID,
		TenantUUID:   reg.TenantUUID,
	})
	require.NoError(t, err)
	require.True(t, result.FallbackUsed, "router should return static fallback payload")
	require.Empty(t, result.AdapterID, "no adapter should be selected when fallback triggers")
	require.NotEmpty(t, result.Payload, "fallback payload should be returned")

	var fallbackPayload map[string]interface{}
	require.NoError(t, json.Unmarshal(result.Payload, &fallbackPayload))
	require.Equal(t, "fallback", fallbackPayload["status"])

	select {
	case evt := <-fallbackEvents:
		require.Equal(t, router.EventRouterFallback, evt.Name)
		payloadMap, ok := evt.Payload.(map[string]interface{})
		require.True(t, ok, "fallback event payload should be a map")
		require.Equal(t, reg.CapabilityID, payloadMap["capability_id"])
		require.Equal(t, reg.TenantUUID, payloadMap["tenant_uuid"])
		require.Equal(t, true, payloadMap["fallback_used"])
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for capability.router.fallback event")
	}
}
