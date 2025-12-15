//go:build ignore

package integrationgatewaycontract

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
	repoig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/tests/integration_gateway/testenv"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	authorization "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
)

const mcpToolTenantUUID = "a5c95015-5b10-4c3a-a056-64042d8d9b68"

func TestIntegrationGatewayMCPTools(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	// 准备租户服务依赖
	inst := instrumentation.NewInstrumentation(nil)
	routerStub := &stubRouter{
		payload: []byte(`{"result":"ok"}`),
	}
	limiter := &mcpStubRateLimiter{
		allowed: true,
	}

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
				Limit:         100,
				Burst:         100,
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

	// 创建包含 mcp 通道的路由
	route, err := env.Service.CreateRoute(context.Background(), manager.CreateRouteInput{
		TenantUUID:   mcpToolTenantUUID,
		Actor:        "test",
		RouteSlug:    "crm-sync",
		CapabilityID: "cap.crm.sync",
		ToolGrantIDs: []string{"grant-crm"},
		Channels:     []string{"http", "mcp"},
		RateLimit: &manager.RateLimitPolicy{
			Limit:         60,
			Burst:         60,
			WindowSeconds: 60,
			Scope:         "per_route_per_tenant",
		},
	})
	require.NoError(t, err)

	// 创建仅 HTTP 通道的路由用于过滤
	_, err = env.Service.CreateRoute(context.Background(), manager.CreateRouteInput{
		TenantUUID:   mcpToolTenantUUID,
		Actor:        "test",
		RouteSlug:    "http-only",
		CapabilityID: "cap.http.only",
		ToolGrantIDs: []string{"grant-http"},
		Channels:     []string{"http"},
	})
	require.NoError(t, err)

	t.Run("list routes should filter by mcp channel and tenant", func(t *testing.T) {
		handler, ok := registry.GetToolHandler("integration.route.list")
		require.True(t, ok)

		result, err := handler(context.Background(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "integration.route.list",
				Arguments: map[string]interface{}{
					"tenant_uuid": mcpToolTenantUUID,
				},
			},
		})
		require.NoError(t, err)

		payload := decodeJSONContent(t, result)
		routes, ok := payload["routes"].([]interface{})
		require.True(t, ok)
		require.Len(t, routes, 1)

		first := routes[0].(map[string]interface{})
		require.Equal(t, "crm-sync", first["route_slug"])
		require.Contains(t, first["channels"], "mcp")
		require.NotEmpty(t, payload["trace_id"])
	})

	t.Run("invoke route success", func(t *testing.T) {
		routerStub.setSuccessPayload([]byte(`{"status":"queued"}`))

		handler, ok := registry.GetToolHandler("integration.route.invoke")
		require.True(t, ok)

		result, err := handler(context.Background(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "integration.route.invoke",
				Arguments: map[string]interface{}{
					"tenant_uuid": mcpToolTenantUUID,
					"route_slug": route.RouteSlug,
					"payload": map[string]interface{}{
						"customer_id": "C123",
					},
				},
			},
		})
		require.NoError(t, err)

		payload := decodeJSONContent(t, result)
		require.Equal(t, "ok", payload["status"])
		require.Equal(t, "cap.crm.sync", payload["routed_capability_id"])
		require.Equal(t, "queued", payload["result"].(map[string]interface{})["status"])
		require.NotEmpty(t, payload["trace_id"])
	})

	t.Run("invoke route failure propagates error", func(t *testing.T) {
		routerStub.setFailure(errors.New("router down"))

		handler, ok := registry.GetToolHandler("integration.route.invoke")
		require.True(t, ok)

		result, err := handler(context.Background(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "integration.route.invoke",
				Arguments: map[string]interface{}{
					"tenant_uuid": mcpToolTenantUUID,
					"route_slug": route.RouteSlug,
					"payload": map[string]interface{}{
						"customer_id": "C999",
					},
				},
			},
		})
		require.NoError(t, err)

		payload := decodeJSONContent(t, result)
		require.Equal(t, "failed", payload["status"])
		require.Contains(t, payload["error_message"], "router down")
		require.NotEmpty(t, payload["trace_id"])
	})
}

func decodeJSONContent(t *testing.T, result *mcp.CallToolResult) map[string]interface{} {
	t.Helper()
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)

	var jsonText string
	for _, content := range result.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			if tc.Type == "text" && jsonText == "" {
				jsonText = tc.Text
			}
		}
	}
	require.NotEmpty(t, jsonText, "expected JSON text content")

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsonText), &payload))
	return payload
}

type stubRouter struct {
	payload []byte
	err     error
}

func (s *stubRouter) setSuccessPayload(p []byte) {
	s.payload = p
	s.err = nil
}

func (s *stubRouter) setFailure(err error) {
	s.err = err
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

type mcpStubRateLimiter struct {
	allowed bool
}

func (s *mcpStubRateLimiter) Allow(_ context.Context, _ string, _ authorization.RateLimitPolicy) (authorization.RateLimitResult, error) {
	if s.allowed {
		return authorization.RateLimitResult{
			Allowed:    true,
			Remaining:  1,
			ResetAfter: time.Minute,
		}, nil
	}
	return authorization.RateLimitResult{
		Allowed:    false,
		Remaining:  0,
		ResetAfter: time.Minute,
	}, nil
}
