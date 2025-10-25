package integrationgatewayintegration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/mcp/register"
	integrationtools "github.com/ArtisanCloud/PowerX/internal/server/mcp/tools/integration_gateway"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	tenantservice "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	modelig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	repoig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/tests/integration_gateway/testenv"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	authorization "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
)

func TestMCPAgentFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	inst := instrumentation.NewInstrumentation(nil)
	routerStub := &stubRouter{
		payload: []byte(`{"pipeline":"started"}`),
	}
	limiter := &mcpStubRateLimiter{allowed: true}

	tenantSvc := tenantservice.NewService(tenantservice.ServiceOptions{
		DB:              env.DB,
		RouteRepo:       repoig.NewIntegrationRouteRepository(env.DB),
		InvocationRepo:  repoig.NewIntegrationInvocationLogRepository(env.DB),
		EventRepo:       repoig.NewIntegrationEventPublicationRepository(env.DB),
		Router:          routerStub,
		RateLimiter:     limiter,
		EventBus:        env.Bus,
		Instrumentation: inst,
		Auditor:         audit.Noop{},
		Config: tenantservice.Config{
			DefaultRateLimit: manager.RateLimitPolicy{
				Limit:         5,
				Burst:         5,
				WindowSeconds: 60,
				Scope:         "per_route_per_tenant",
			},
			EventTopics: manager.EventTopics{
				InvocationSucceeded: "integration.gateway.invocation.succeeded",
				InvocationFailed:    "integration.gateway.invocation.failed",
			},
		},
		Clock: time.Now,
	})

	registry := register.NewToolRegistry(nil)
	err := integrationtools.RegisterToolsWithRegistry(registry, integrationtools.ToolDependencies{
		TenantService:   tenantSvc,
		ManagerService:  env.Service,
		Instrumentation: inst,
	})
	require.NoError(t, err)

	_, err = env.Service.CreateRoute(context.Background(), manager.CreateRouteInput{
		TenantID:     "tenant-001",
		Actor:        "integration-test",
		RouteSlug:    "agent-flow",
		CapabilityID: "cap.agent.flow",
		ToolGrantIDs: []string{"grant-agent"},
		Channels:     []string{"mcp"},
	})
	require.NoError(t, err)

	listHandler, ok := registry.GetToolHandler("integration.route.list")
	require.True(t, ok)
	listResult, err := listHandler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "integration.route.list",
			Arguments: map[string]interface{}{
				"tenant_id": "tenant-001",
			},
		},
	})
	require.NoError(t, err)
	payload := decodeJSONContent(t, listResult)
	require.Len(t, payload["routes"].([]interface{}), 1)

	invokeHandler, ok := registry.GetToolHandler("integration.route.invoke")
	require.True(t, ok)
	successResult, err := invokeHandler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "integration.route.invoke",
			Arguments: map[string]interface{}{
				"tenant_id":  "tenant-001",
				"route_slug": "agent-flow",
				"payload": map[string]interface{}{
					"job": "sync",
				},
			},
		},
	})
	require.NoError(t, err)
	successPayload := decodeJSONContent(t, successResult)
	require.Equal(t, "ok", successPayload["status"])

	routerStub.setFailure(errors.New("capability unavailable"))
	failResult, err := invokeHandler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "integration.route.invoke",
			Arguments: map[string]interface{}{
				"tenant_id":  "tenant-001",
				"route_slug": "agent-flow",
				"payload": map[string]interface{}{
					"job": "sync",
				},
			},
		},
	})
	require.NoError(t, err)
	failPayload := decodeJSONContent(t, failResult)
	require.Equal(t, "failed", failPayload["status"])
	require.Contains(t, failPayload["error_message"], "capability unavailable")

	var invocationCount int64
	require.NoError(t, env.DB.Model(&modelig.IntegrationInvocationLog{}).Count(&invocationCount).Error)
	require.EqualValues(t, 2, invocationCount)

	var eventCount int64
	require.NoError(t, env.DB.Model(&modelig.IntegrationEventPublication{}).Count(&eventCount).Error)
	require.EqualValues(t, 2, eventCount)
}

func decodeJSONContent(t *testing.T, result *mcp.CallToolResult) map[string]interface{} {
	t.Helper()
	require.NotNil(t, result)

	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok && textContent.Type == "text" {
			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(textContent.Text), &payload))
			return payload
		}
	}
	t.Fatalf("no JSON text content found")
	return nil
}

type stubRouter struct {
	payload []byte
	err     error
}

func (s *stubRouter) Invoke(_ context.Context, _ router.InvokeRequest) (router.InvokeResult, error) {
	if s.err != nil {
		return router.InvokeResult{
			AdapterID: "router-stub",
			Error:     s.err,
		}, nil
	}
	return router.InvokeResult{
		AdapterID: "router-stub",
		Payload:   s.payload,
	}, nil
}

func (s *stubRouter) setFailure(err error) {
	s.err = err
}

type mcpStubRateLimiter struct {
	allowed bool
}

func (s *mcpStubRateLimiter) Allow(_ context.Context, _ string, _ authorization.RateLimitPolicy) (authorization.RateLimitResult, error) {
	if s.allowed {
		return authorization.RateLimitResult{Allowed: true, Remaining: 1, ResetAfter: time.Minute}, nil
	}
	return authorization.RateLimitResult{Allowed: false, Remaining: 0, ResetAfter: time.Minute}, nil
}
