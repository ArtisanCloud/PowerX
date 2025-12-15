//go:build ignore

package integrationgatewayintegration

import (
	"context"
	"testing"

	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	modelig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/tests/integration_gateway/testenv"
	"github.com/stretchr/testify/require"
)

const adminFlowTenantUUID = "7f8791a7-84de-4613-92d5-46e0f607ddb9"

func TestAdminManagementFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	route, err := env.Service.CreateRoute(ctx, manager.CreateRouteInput{
		TenantUUID:   adminFlowTenantUUID,
		Actor:        "integration-test",
		RouteSlug:    "integration-route",
		CapabilityID: "cap.integration",
		ToolGrantIDs: []string{"grant-1"},
		Channels:     []string{"http"},
	})
	require.NoError(t, err)
	require.Equal(t, uint32(1), route.CurrentVersion)

	updated, err := env.Service.UpdateRoute(ctx, manager.UpdateRouteInput{
		RouteID:     route.RouteID,
		TenantUUID:  adminFlowTenantUUID,
		Actor:       "integration-test",
		Version:     route.CurrentVersion,
		Channels:    []string{"http", "mcp"},
		Description: pointer("updated"),
	})
	require.NoError(t, err)
	require.Equal(t, uint32(2), updated.CurrentVersion)
	require.ElementsMatch(t, []string{"http", "mcp"}, updated.Channels)

	resumed, err := env.Service.ChangeLifecycle(ctx, manager.ChangeLifecycleInput{
		RouteID:    route.RouteID,
		TenantUUID: adminFlowTenantUUID,
		Actor:    "integration-test",
		Action:   "suspend",
	})
	require.NoError(t, err)
	require.Equal(t, manager.LifecycleSuspended, resumed.LifecycleState)

	_, err = env.Service.ChangeLifecycle(ctx, manager.ChangeLifecycleInput{
		RouteID:    route.RouteID,
		TenantUUID: adminFlowTenantUUID,
		Actor:    "integration-test",
		Action:   "resume",
	})
	require.NoError(t, err)

	versions, err := env.Service.ListVersions(ctx, route.RouteID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(versions), 3)

	var events []modelig.IntegrationEventPublication
	require.NoError(t, env.DB.Find(&events).Error)
	require.NotEmpty(t, events)
}

func pointer[T any](v T) *T { return &v }
