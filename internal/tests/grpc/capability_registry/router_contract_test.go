package capabilityregistry

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/test/bufconn"

	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistryHealth "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/health"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/ArtisanCloud/PowerX/internal/tests/capability_registry/testutil"
	capabilityRegistryGRPC "github.com/ArtisanCloud/PowerX/internal/transport/grpc/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

const routerBufSize = 1024 * 1024

func TestCapabilityRouterGRPCContracts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	env := newRouterGRPCTestEnv(t)
	t.Cleanup(env.Close)

	// happy path should route to primary adapter
	invokeResp, err := env.client.Invoke(ctx, &capabilityRegistryPB.InvokeRequest{
		Capability: &capabilityRegistryPB.TenantScopedId{
			CapabilityId: "capabilities.text.translate",
			TenantId:     "tenant-corex",
		},
	})
	assertNoError(t, err)
	assertEqual(t, "adapter-primary", invokeResp.GetAdapterId(), "primary adapter selection")
	assertBoolFalseRouter(t, invokeResp.GetFallbackUsed(), "primary fallback flag")

	// mark primary unhealthy, expect backup
	_, err = env.client.ReportHealth(ctx, &capabilityRegistryPB.ReportHealthRequest{
		Id: &capabilityRegistryPB.TenantScopedId{
			CapabilityId: "capabilities.text.translate",
			TenantId:     "tenant-corex",
		},
		AdapterId: "adapter-primary",
		Status:    "unhealthy",
		Reason:    "timeout",
		Failures:  3,
	})
	assertNoError(t, err)

	invokeResp2, err := env.client.Invoke(ctx, &capabilityRegistryPB.InvokeRequest{
		Capability: &capabilityRegistryPB.TenantScopedId{
			CapabilityId: "capabilities.text.translate",
			TenantId:     "tenant-corex",
		},
	})
	assertNoError(t, err)
	assertEqual(t, "adapter-backup", invokeResp2.GetAdapterId(), "backup adapter selection")

	// mark backup unhealthy -> expect fallback static response
	_, err = env.client.ReportHealth(ctx, &capabilityRegistryPB.ReportHealthRequest{
		Id: &capabilityRegistryPB.TenantScopedId{
			CapabilityId: "capabilities.text.translate",
			TenantId:     "tenant-corex",
		},
		AdapterId: "adapter-backup",
		Status:    "unhealthy",
		Reason:    "circuit-open",
		Failures:  2,
	})
	assertNoError(t, err)

	invokeResp3, err := env.client.Invoke(ctx, &capabilityRegistryPB.InvokeRequest{
		Capability: &capabilityRegistryPB.TenantScopedId{
			CapabilityId: "capabilities.text.translate",
			TenantId:     "tenant-corex",
		},
	})
	assertNoError(t, err)
	assertBoolTrueRouter(t, invokeResp3.GetFallbackUsed(), "fallback flag")
	assertEqual(t, "", invokeResp3.GetAdapterId(), "no adapter when fallback triggered")
	assertContainsStringRouter(t, string(invokeResp3.GetPayload()), "fallback", "static fallback payload")

	// report unknown capability should return error
	_, err = env.client.Invoke(ctx, &capabilityRegistryPB.InvokeRequest{
		Capability: &capabilityRegistryPB.TenantScopedId{
			CapabilityId: "capabilities.unknown",
			TenantId:     "tenant-corex",
		},
	})
	assertStatusCode(t, codes.NotFound, err)
}

// --- GRPC test environment -------------------------------------------------

type routerGRPCTestEnv struct {
	listener *bufconn.Listener
	server   *grpc.Server
	client   capabilityRegistryPB.CapabilityRouterServiceClient
}

func newRouterGRPCTestEnv(t *testing.T) *routerGRPCTestEnv {
	listener := bufconn.Listen(routerBufSize)
	server := grpc.NewServer()

	registryRepo := testutil.NewMockRegistryRepository([]router.Registration{
		{
			CapabilityID: "capabilities.text.translate",
			TenantID:     "tenant-corex",
			Status:       "published",
			Adapters: []router.AdapterEndpoint{
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
			RoutingPolicy: router.RoutingPolicy{
				Strategy:        "weighted_round_robin",
				CooldownSeconds: 60,
			},
			FallbackPlan: &router.FallbackPlan{
				FallbackTargets: []string{},
				StaticResponse: &router.StaticResponse{
					Payload: map[string]interface{}{
						"message": "fallback-static",
					},
					TTLSeconds: 60,
				},
			},
		},
	})
	eventBus := event_bus.NewLocalEventBus()
	routerSvc := router.NewService(router.ServiceOptions{
		RegistryRepository: registryRepo,
		HealthRepository:   capabilityRegistryHealth.NewMemoryRepository(),
		EventBus:           eventBus,
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
		Clock: func() time.Time {
			return time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
		},
	})

	capabilityRegistryGRPC.RegisterCapabilityRouterServer(server, routerSvc)

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Logf("router grpc server stopped: %v", err)
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
	assertNoError(t, err)

	client := capabilityRegistryPB.NewCapabilityRouterServiceClient(conn)

	return &routerGRPCTestEnv{
		listener: listener,
		server:   server,
		client:   client,
	}
}

func (env *routerGRPCTestEnv) Close() {
	env.server.GracefulStop()
	env.listener.Close()
}

// --- Assertions ------------------------------------------------------------

func assertContainsStringRouter(t *testing.T, content, substr, field string) {
	t.Helper()
	if !strings.Contains(content, substr) {
		t.Fatalf("expected %s to contain %q, got %q", field, substr, content)
	}
}

func assertBoolTrueRouter(t *testing.T, cond bool, field string) {
	t.Helper()
	if !cond {
		t.Fatalf("expected %s to be true", field)
	}
}

func assertBoolFalseRouter(t *testing.T, cond bool, field string) {
	t.Helper()
	if cond {
		t.Fatalf("expected %s to be false", field)
	}
}

// --- Mock registry repository ---------------------------------------------
