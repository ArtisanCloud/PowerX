package integrationgatewaycontract

import (
	"context"
	"net"
	"testing"
	"time"

	pbintegration "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration_gateway/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	authorization "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	tenant "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
	grpcintegration "github.com/ArtisanCloud/PowerX/internal/transport/grpc/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	repoig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/tests/integration_gateway/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

func TestTenantGRPCWorkflow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	route, err := env.Service.CreateRoute(ctx, manager.CreateRouteInput{
		TenantID:     "tenant-grpc",
		Actor:        "tenant-grpc-test",
		RouteSlug:    "grpc-sync",
		CapabilityID: "cap.grpc.sync",
		ToolGrantIDs: []string{"grant-grpc"},
		Channels:     []string{"http"},
	})
	require.NoError(t, err)

	routerStub := &httpStubRouter{
		response: router.InvokeResult{
			AdapterID: "adapter-grpc",
			Payload:   []byte(`{"value":"grpc"}`),
			Latency:   10 * time.Millisecond,
		},
	}
	limiter := &stubRateLimiter{
		result: authorization.RateLimitResult{
			Allowed:    true,
			Remaining:  5,
			ResetAfter: 45 * time.Second,
		},
	}

	inst := instrumentation.NewInstrumentation(nil)
	tenantSvc := tenant.NewService(tenant.ServiceOptions{
		DB:              env.DB,
		RouteRepo:       repoig.NewIntegrationRouteRepository(env.DB),
		InvocationRepo:  repoig.NewIntegrationInvocationLogRepository(env.DB),
		EventRepo:       repoig.NewIntegrationEventPublicationRepository(env.DB),
		Router:          routerStub,
		RateLimiter:     limiter,
		EventBus:        env.Bus,
		Instrumentation: inst,
		Auditor:         audit.Noop{},
		Config: tenant.Config{
			EventTopics: manager.EventTopics{
				InvocationSucceeded: "integration.gateway.invocation.succeeded",
				InvocationFailed:    "integration.gateway.invocation.failed",
			},
			DefaultRateLimit: manager.RateLimitPolicy{
				Limit:         100,
				Burst:         100,
				WindowSeconds: 60,
				Scope:         "per_route_per_tenant",
			},
		},
		Clock: time.Now,
	})

	deps := &shared.Deps{
		EventBus: env.Bus,
		IntegrationGateway: &shared.IntegrationGatewayDeps{
			Manager:         env.Service,
			Tenant:          tenantSvc,
			Instrumentation: inst,
		},
	}

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	grpcintegration.RegisterServers(server, deps)
	done := make(chan struct{})
	go func() {
		_ = server.Serve(listener)
		close(done)
	}()
	t.Cleanup(func() {
		server.GracefulStop()
		_ = listener.Close()
		<-done
	})

	dialer := func(context.Context, string) (net.Conn, error) { return listener.Dial() }
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(dialer), grpc.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := pbintegration.NewIntegrationGatewayTenantServiceClient(conn)

	listResp, err := client.ListRoutes(ctx, &pbintegration.TenantListRoutesRequest{
		TenantId: "tenant-grpc",
	})
	require.NoError(t, err)
	require.NotEmpty(t, listResp.Items)

	getResp, err := client.GetRoute(ctx, &pbintegration.TenantGetRouteRequest{
		TenantId:  "tenant-grpc",
		RouteSlug: "grpc-sync",
	})
	require.NoError(t, err)
	require.Equal(t, route.RouteSlug, getResp.Route.RouteSlug)

	invokeResp, err := client.InvokeRoute(ctx, &pbintegration.TenantInvokeRequest{
		TenantId:    "tenant-grpc",
		RouteSlug:   "grpc-sync",
		PayloadJson: []byte(`{"action":"test"}`),
	})
	require.NoError(t, err)
	require.Equal(t, pbintegration.TenantInvokeResponse_OK, invokeResp.Status)
	require.Contains(t, string(invokeResp.ResultJson), "grpc")

	limiter.limit = 1
	secondResp, err := client.InvokeRoute(ctx, &pbintegration.TenantInvokeRequest{
		TenantId:    "tenant-grpc",
		RouteSlug:   "grpc-sync",
		PayloadJson: []byte(`{"action":"again"}`),
	})
	require.NoError(t, err)
	require.Equal(t, pbintegration.TenantInvokeResponse_RATE_LIMITED, secondResp.Status)
}
