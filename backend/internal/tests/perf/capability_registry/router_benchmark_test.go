package capabilityregistry

import (
	"context"
	"testing"
	"time"

	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistryHealth "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/health"
	capabilityRegistryRegistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/ArtisanCloud/PowerX/internal/tests/capability_registry/testutil"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

func BenchmarkRouterInvokePrimary(b *testing.B) {
	ctx := context.Background()
	registryRepo := testutil.NewMockRegistryRepository([]router.Registration{
		{
			CapabilityID: "capabilities.text.translate",
			TenantUUID:   "tenant-corex",
			Status:       "published",
			Adapters: []router.AdapterEndpoint{
				{
					AdapterID:     "adapter-primary",
					TransportType: "grpc",
					Endpoint:      "grpc://translator.corex.svc:443",
					Weight:        80,
					TimeoutMS:     2000,
				},
				{
					AdapterID:     "adapter-backup",
					TransportType: "http",
					Endpoint:      "https://translator.corex/api",
					Weight:        20,
					TimeoutMS:     2500,
				},
			},
			RoutingPolicy: router.RoutingPolicy{
				Strategy:        "priority",
				CooldownSeconds: 60,
			},
			FallbackPlan: &router.FallbackPlan{
				StaticResponse: &router.StaticResponse{
					Payload: map[string]interface{}{"message": "fallback"},
				},
			},
		},
	})

	routerSvc := router.NewService(router.ServiceOptions{
		RegistryRepository: registryRepo,
		HealthRepository:   capabilityRegistryHealth.NewMemoryRepository(),
		EventBus:           event_bus.NewLocalEventBus(),
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
	})
	req := router.InvokeRequest{
		CapabilityID: "capabilities.text.translate",
		TenantUUID:   "tenant-corex",
	}

	if _, err := routerSvc.Invoke(ctx, req); err != nil {
		b.Fatalf("initial invoke failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := routerSvc.Invoke(ctx, req); err != nil {
			b.Fatalf("invoke failed: %v", err)
		}
	}
}

func BenchmarkRouterFallback(b *testing.B) {
	ctx := context.Background()
	registryRepo := testutil.NewMockRegistryRepository([]router.Registration{
		{
			CapabilityID: "capabilities.text.translate",
			TenantUUID:   "tenant-corex",
			Status:       "published",
			Adapters: []router.AdapterEndpoint{
				{
					AdapterID:     "adapter-primary",
					TransportType: "grpc",
					Endpoint:      "grpc://translator.corex.svc:443",
					Weight:        80,
					TimeoutMS:     2000,
				},
			},
			RoutingPolicy: router.RoutingPolicy{
				Strategy:        "priority",
				CooldownSeconds: 60,
			},
			FallbackPlan: &router.FallbackPlan{
				StaticResponse: &router.StaticResponse{
					Payload: map[string]interface{}{"message": "fallback"},
				},
			},
		},
	})

	routerSvc := router.NewService(router.ServiceOptions{
		RegistryRepository: registryRepo,
		HealthRepository:   capabilityRegistryHealth.NewMemoryRepository(),
		EventBus:           event_bus.NewLocalEventBus(),
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
	})
	req := router.InvokeRequest{
		CapabilityID: "capabilities.text.translate",
		TenantUUID:   "tenant-corex",
	}

	// 标记适配器为不可用以触发 fallback。
	if err := routerSvc.ReportHealth(ctx, router.ReportHealthInput{
		CapabilityID: "capabilities.text.translate",
		TenantUUID:   "tenant-corex",
		AdapterID:    "adapter-primary",
		Status:       "unhealthy",
		Reason:       "benchmark",
	}); err != nil {
		b.Fatalf("report health failed: %v", err)
	}

	if _, err := routerSvc.Invoke(ctx, req); err != nil {
		b.Fatalf("initial fallback invoke failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := routerSvc.Invoke(ctx, req); err != nil {
			b.Fatalf("fallback invoke failed: %v", err)
		}
	}
}

func BenchmarkRegistryGetLatest(b *testing.B) {
	ctx := context.Background()
	registration := router.Registration{
		CapabilityID: "capabilities.text.translate",
		TenantUUID:   "tenant-corex",
		Status:       "published",
		Version:      5,
		Adapters: []router.AdapterEndpoint{
			{
				AdapterID:     "adapter-primary",
				TransportType: "grpc",
				Endpoint:      "grpc://translator.corex.svc:443",
				Weight:        80,
				TimeoutMS:     2000,
			},
		},
		RoutingPolicy: router.RoutingPolicy{
			Strategy:        "priority",
			CooldownSeconds: 60,
		},
	}

	repo := testutil.NewMockRegistryRepository([]router.Registration{registration})
	registrySvc := capabilityRegistryRegistry.NewService(capabilityRegistryRegistry.ServiceOptions{
		Repository:      repo,
		EventBus:        event_bus.NewLocalEventBus(),
		Instrumentation: capabilityRegistryDomain.NewInstrumentation(nil),
		Clock:           time.Now,
	})

	opts := capabilityRegistryRegistry.GetRegistrationOptions{VersionSelector: "latest"}
	if _, err := registrySvc.GetRegistration(ctx, registration.CapabilityID, registration.TenantUUID, opts); err != nil {
		b.Fatalf("initial get failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := registrySvc.GetRegistration(ctx, registration.CapabilityID, registration.TenantUUID, opts); err != nil {
			b.Fatalf("get registration failed: %v", err)
		}
	}
}
