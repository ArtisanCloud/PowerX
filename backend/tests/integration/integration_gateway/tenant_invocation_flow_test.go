//go:build ignore

package integrationgatewayintegration

import (
	"context"
	"sync"
	"testing"
	"time"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	authorization "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	tenant "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	modelig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	repoig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/tests/integration_gateway/testenv"
	"github.com/stretchr/testify/require"
)

const tenantFlowUUID = "35e8a2b2-38f5-4e7f-9075-0bc96d0e1907"

func TestTenantInvocationFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	_, err := env.Service.CreateRoute(ctx, manager.CreateRouteInput{
		TenantUUID:   tenantFlowUUID,
		Actor:        "tenant-flow-test",
		RouteSlug:    "flow-sync",
		CapabilityID: "cap.flow.sync",
		ToolGrantIDs: []string{"grant-flow"},
		Channels:     []string{"http"},
	})
	require.NoError(t, err)

	routerStub := &integrationRouterStub{
		payload: []byte(`{"status":"ok"}`),
	}
	limiter := &integrationRateLimiter{
		result: authorization.RateLimitResult{
			Allowed:    true,
			Remaining:  3,
			ResetAfter: 60 * time.Second,
		},
	}

	invokeRepo := repoig.NewIntegrationInvocationLogRepository(env.DB)
	eventRepo := repoig.NewIntegrationEventPublicationRepository(env.DB)
	tenantSvc := tenant.NewService(tenant.ServiceOptions{
		DB:             env.DB,
		RouteRepo:      repoig.NewIntegrationRouteRepository(env.DB),
		InvocationRepo: invokeRepo,
		EventRepo:      eventRepo,
		Router:         routerStub,
		RateLimiter:    limiter,
		EventBus:       env.Bus,
		Auditor:        audit.Noop{},
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

	var (
		wg     sync.WaitGroup
		events int
	)
	wg.Add(1)
	unsub := env.Bus.Subscribe("integration.gateway.invocation.succeeded", func(evt event_bus.Event) error {
		defer wg.Done()
		events++
		return nil
	})
	defer unsub()

	result, err := tenantSvc.Invoke(ctx, tenant.InvokeInput{
		TenantUUID: tenantFlowUUID,
		RouteSlug: "flow-sync",
		Payload:   map[string]any{"order_id": "o-1"},
	})
	require.NoError(t, err)
	require.Equal(t, "cap.flow.sync", result.RoutedCapabilityID)

	wg.Wait()
	require.Equal(t, 1, events)

	var logsCount int64
	require.NoError(t, env.DB.Model(&modelig.IntegrationInvocationLog{}).Count(&logsCount).Error)
	require.EqualValues(t, 1, logsCount)
}

type integrationRouterStub struct {
	payload []byte
}

func (s *integrationRouterStub) Invoke(_ context.Context, in router.InvokeRequest) (router.InvokeResult, error) {
	return router.InvokeResult{
		AdapterID:    "adapter-flow",
		Endpoint:     "mock://flow",
		Transport:    "http",
		FallbackUsed: false,
		Payload:      s.payload,
		Latency:      15 * time.Millisecond,
	}, nil
}

type integrationRateLimiter struct {
	result authorization.RateLimitResult
}

func (r *integrationRateLimiter) Allow(context.Context, string, authorization.RateLimitPolicy) (authorization.RateLimitResult, error) {
	return r.result, nil
}
