package integrationgatewaycontract

import (
	"context"
	"net"
	"testing"
	"time"

	pbintegration "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration/gateway/v1"
	grpcintegration "github.com/ArtisanCloud/PowerX/internal/transport/grpc/integration_gateway"
	"github.com/ArtisanCloud/PowerX/tests/integration_gateway/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

func TestAdminGRPCWorkflow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	grpcintegration.RegisterServers(server, env.Deps)
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

	dialer := func(ctx context.Context, s string) (net.Conn, error) {
		return listener.Dial()
	}

	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := pbintegration.NewIntegrationGatewayAdminServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	createResp, err := client.CreateRoute(ctx, &pbintegration.CreateRouteRequest{
		TenantId:     "tenant-001",
		RouteSlug:    "grpc-sync",
		CapabilityId: "cap.grpc.sync",
		ToolGrantIds: []string{"grant-grpc"},
	})
	require.NoError(t, err)
	require.NotNil(t, createResp)
	require.NotEmpty(t, createResp.Route.RouteId)

	routeID := createResp.Route.RouteId

	getResp, err := client.GetRoute(ctx, &pbintegration.GetRouteRequest{RouteId: routeID})
	require.NoError(t, err)
	require.Equal(t, routeID, getResp.Route.RouteId)

	updateResp, err := client.UpdateRoute(ctx, &pbintegration.UpdateRouteRequest{
		RouteId:       routeID,
		ExpectVersion: createResp.Route.CurrentVersion,
		Channels:      []string{"http", "mcp"},
		Description:   "grpc update",
	})
	require.NoError(t, err)
	require.Contains(t, updateResp.Route.Channels, "mcp")

	_, err = client.ChangeLifecycle(ctx, &pbintegration.ChangeLifecycleRequest{
		RouteId:     routeID,
		TargetState: pbintegration.IntegrationRoute_SUSPENDED,
		Reason:      "maintenance",
	})
	require.NoError(t, err)

	_, err = client.ChangeLifecycle(ctx, &pbintegration.ChangeLifecycleRequest{
		RouteId:     routeID,
		TargetState: pbintegration.IntegrationRoute_ACTIVE,
		Reason:      "resume",
	})
	require.NoError(t, err)

	_, err = client.ChangeLifecycle(ctx, &pbintegration.ChangeLifecycleRequest{
		RouteId:     routeID,
		TargetState: pbintegration.IntegrationRoute_RETIRED,
	})
	require.NoError(t, err)

	listResp, err := client.ListRoutes(ctx, &pbintegration.ListRoutesRequest{TenantId: "tenant-001"})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listResp.Items), 1)

	versionsResp, err := client.ListRouteVersions(ctx, &pbintegration.ListRouteVersionsRequest{RouteId: routeID})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(versionsResp.Versions), 2)
}
