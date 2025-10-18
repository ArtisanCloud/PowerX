package capabilityregistry

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistryHealth "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/health"
	routerService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	sandboxService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/sandbox"
	"github.com/ArtisanCloud/PowerX/internal/tests/capability_registry/testutil"
	capabilityRegistryGRPC "github.com/ArtisanCloud/PowerX/internal/transport/grpc/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

func TestCapabilityRouterSandboxSimulate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	env := newRouterSandboxTestEnv(t)
	t.Cleanup(env.Close)

	resp, err := env.client.Simulate(ctx, &capabilityRegistryPB.SandboxInvokeRequest{
		Request: &capabilityRegistryPB.InvokeRequest{
			Capability: &capabilityRegistryPB.TenantScopedId{
				CapabilityId: "capabilities.text.translate",
				TenantId:     "tenant-corex",
			},
		},
	})
	if err != nil {
		t.Fatalf("simulate invoke failed: %v", err)
	}
	if resp.GetResponse().GetAdapterId() != "adapter-primary" {
		t.Fatalf("expected primary adapter, got %s", resp.GetResponse().GetAdapterId())
	}

	err = env.router.ReportHealth(ctx, routerService.ReportHealthInput{
		CapabilityID: "capabilities.text.translate",
		TenantID:     "tenant-corex",
		AdapterID:    "adapter-primary",
		Status:       "unhealthy",
	})
	if err != nil {
		t.Fatalf("report health failed: %v", err)
	}

	resp2, err := env.client.Simulate(ctx, &capabilityRegistryPB.SandboxInvokeRequest{
		Request: &capabilityRegistryPB.InvokeRequest{
			Capability: &capabilityRegistryPB.TenantScopedId{
				CapabilityId: "capabilities.text.translate",
				TenantId:     "tenant-corex",
			},
		},
	})
	if err != nil {
		t.Fatalf("simulate fallback failed: %v", err)
	}
	if resp2.GetResponse().GetAdapterId() != "adapter-backup" {
		t.Fatalf("expected backup adapter, got %s", resp2.GetResponse().GetAdapterId())
	}
}

type routerSandboxTestEnv struct {
	listener *bufconn.Listener
	server   *grpc.Server
	client   capabilityRegistryPB.CapabilityRouterSandboxServiceClient
	router   *routerService.Service
}

func newRouterSandboxTestEnv(t *testing.T) *routerSandboxTestEnv {
	listener := bufconn.Listen(routerBufSize)
	server := grpc.NewServer()

	registryRepo := testutil.NewMockRegistryRepository([]routerService.Registration{
		{
			CapabilityID: "capabilities.text.translate",
			TenantID:     "tenant-corex",
			Status:       "published",
			Adapters: []routerService.AdapterEndpoint{
				{
					AdapterID:     "adapter-primary",
					TransportType: "grpc",
					Endpoint:      "grpc://translator.corex.svc:443",
					Weight:        80,
					TimeoutMS:     4000,
				},
				{
					AdapterID:     "adapter-backup",
					TransportType: "http",
					Endpoint:      "https://translator.corex/api",
					Weight:        20,
					TimeoutMS:     3000,
				},
			},
			RoutingPolicy: routerService.RoutingPolicy{
				Strategy:        "weighted_round_robin",
				CooldownSeconds: 60,
			},
			FallbackPlan: &routerService.FallbackPlan{
				FallbackTargets: []string{},
			},
		},
	})
	routerSvc := routerService.NewService(routerService.ServiceOptions{
		RegistryRepository: registryRepo,
		HealthRepository:   capabilityRegistryHealth.NewMemoryRepository(),
		EventBus:           event_bus.NewLocalEventBus(),
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
		Clock: func() time.Time {
			return time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
		},
	})
	sandboxSvc := sandboxService.NewService(sandboxService.ServiceOptions{
		RegistryRepository: registryRepo,
		RouterService:      routerSvc,
	})

	capabilityRegistryGRPC.RegisterCapabilityRouterSandboxServer(server, sandboxSvc)

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Logf("sandbox grpc server stopped: %v", err)
		}
	}()

	conn, err := grpc.DialContext(
		context.Background(),
		"bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithInsecure(),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("dial sandbox server failed: %v", err)
	}

	client := capabilityRegistryPB.NewCapabilityRouterSandboxServiceClient(conn)

	return &routerSandboxTestEnv{
		listener: listener,
		server:   server,
		client:   client,
		router:   routerSvc,
	}
}

func (env *routerSandboxTestEnv) Close() {
	env.server.GracefulStop()
	env.listener.Close()
}
