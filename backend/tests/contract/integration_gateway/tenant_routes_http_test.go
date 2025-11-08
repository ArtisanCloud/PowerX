package integrationgatewaycontract

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	authorization "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	tenant "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	modelig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	repoig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/tests/integration_gateway/testenv"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type httpStubRouter struct {
	mu       sync.Mutex
	response router.InvokeResult
	err      error
	calls    []router.InvokeRequest
}

func (s *httpStubRouter) Invoke(ctx context.Context, req router.InvokeRequest) (router.InvokeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	return s.response, s.err
}

type stubRateLimiter struct {
	mu      sync.Mutex
	allowed int
	limit   int
	result  authorization.RateLimitResult
}

func (s *stubRateLimiter) Allow(ctx context.Context, _ string, _ authorization.RateLimitPolicy) (authorization.RateLimitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowed++
	if s.limit > 0 && s.allowed > s.limit {
		return authorization.RateLimitResult{
			Allowed:    false,
			Remaining:  0,
			ResetAfter: 30 * time.Second,
		}, nil
	}
	return s.result, nil
}

func setupTenantService(t *testing.T, db *gorm.DB, bus event_bus.EventBus, inst *instrumentation.Instrumentation, routerSvc *httpStubRouter, limiter *stubRateLimiter) *tenant.Service {
	t.Helper()

	routeRepo := repoig.NewIntegrationRouteRepository(db)
	invokeRepo := repoig.NewIntegrationInvocationLogRepository(db)
	eventRepo := repoig.NewIntegrationEventPublicationRepository(db)

	return tenant.NewService(tenant.ServiceOptions{
		DB:              db,
		RouteRepo:       routeRepo,
		InvocationRepo:  invokeRepo,
		EventRepo:       eventRepo,
		Router:          routerSvc,
		RateLimiter:     limiter,
		EventBus:        bus,
		Instrumentation: inst,
		Auditor:         audit.Noop{},
		Config: tenant.Config{
			EventTopics: manager.EventTopics{
				InvocationSucceeded: "integration.gateway.invocation.succeeded",
				InvocationFailed:    "integration.gateway.invocation.failed",
			},
			DefaultRateLimit: manager.RateLimitPolicy{
				Limit:         60,
				Burst:         60,
				WindowSeconds: 60,
				Scope:         "per_route_per_tenant",
			},
		},
		Clock: time.Now,
	})
}

func TestTenantHTTPWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	route, err := env.Service.CreateRoute(ctx, manager.CreateRouteInput{
		TenantID:     "tenant-001",
		Actor:        "tenant-test",
		RouteSlug:    "crm-sync",
		CapabilityID: "cap.crm.sync",
		ToolGrantIDs: []string{"grant-crm"},
		Channels:     []string{"http"},
	})
	require.NoError(t, err)

	routerStub := &httpStubRouter{
		response: router.InvokeResult{
			AdapterID:    "adapter-1",
			Endpoint:     "mock://endpoint",
			Transport:    "http",
			FallbackUsed: false,
			Payload:      []byte(`{"result":"ok"}`),
			Latency:      25 * time.Millisecond,
		},
	}
	limiter := &stubRateLimiter{
		result: authorization.RateLimitResult{
			Allowed:    true,
			Remaining:  10,
			ResetAfter: 60 * time.Second,
		},
		limit: 2,
	}

	inst := instrumentation.NewInstrumentation(nil)
	tenantSvc := setupTenantService(t, env.DB, env.Bus, inst, routerStub, limiter)

	deps := &shared.Deps{
		EventBus: env.Bus,
		IntegrationGateway: &shared.IntegrationGatewayDeps{
			Manager:         env.Service,
			Tenant:          tenantSvc,
			Instrumentation: inst,
		},
	}

	engine := gin.New()
	protected := engine.Group("/api")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	integration_gateway.RegisterTenantRoutes(protected, deps)

	// list routes
	listReq := httptest.NewRequest(http.MethodGet, "/api/tenant/integration/routes", nil)
	listReq.Header.Set("Authorization", "Bearer tenant")
	listReq.Header.Set("X-PowerX-Tenant", "tenant-001")
	listResp := httptest.NewRecorder()
	engine.ServeHTTP(listResp, listReq)
	require.Equal(t, http.StatusOK, listResp.Code)

	var listPayload struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				RouteSlug    string `json:"route_slug"`
				CapabilityID string `json:"capability_id"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listPayload))
	require.Equal(t, http.StatusOK, listPayload.Code)
	require.NotEmpty(t, listPayload.Data.Items)

	// get route
	getReq := httptest.NewRequest(http.MethodGet, "/api/tenant/integration/routes/crm-sync", nil)
	getReq.Header.Set("Authorization", "Bearer tenant")
	getReq.Header.Set("X-PowerX-Tenant", "tenant-001")
	getResp := httptest.NewRecorder()
	engine.ServeHTTP(getResp, getReq)
	require.Equal(t, http.StatusOK, getResp.Code)

	var getPayload struct {
		Code int `json:"code"`
		Data struct {
			RouteSlug string `json:"route_slug"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &getPayload))
	require.Equal(t, "crm-sync", getPayload.Data.RouteSlug)

	// invoke success
	invokeBody := map[string]any{
		"payload": map[string]any{
			"customer_id": "c-001",
		},
	}
	bodyBytes, _ := json.Marshal(invokeBody)
	invokeReq := httptest.NewRequest(http.MethodPost, "/api/tenant/integration/routes/crm-sync/invoke", bytes.NewBuffer(bodyBytes))
	invokeReq.Header.Set("Authorization", "Bearer tenant")
	invokeReq.Header.Set("Content-Type", "application/json")
	invokeReq.Header.Set("X-PowerX-Tenant", "tenant-001")
	invokeResp := httptest.NewRecorder()
	engine.ServeHTTP(invokeResp, invokeReq)
	require.Equal(t, http.StatusOK, invokeResp.Code)

	var invokePayload struct {
		Code int `json:"code"`
		Data struct {
			Result map[string]any `json:"result"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(invokeResp.Body.Bytes(), &invokePayload))
	require.Equal(t, http.StatusOK, invokePayload.Code)
	require.Equal(t, "ok", invokePayload.Data.Result["result"])

	// third call should hit rate limiter
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/tenant/integration/routes/crm-sync/invoke", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Authorization", "Bearer tenant")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-PowerX-Tenant", "tenant-001")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if i == 0 {
			require.Equal(t, http.StatusOK, resp.Code)
		} else {
			require.Equal(t, http.StatusTooManyRequests, resp.Code)
			var rateLimited struct {
				Code int `json:"code"`
			}
			require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &rateLimited))
			require.Equal(t, http.StatusTooManyRequests, rateLimited.Code)
		}
	}

	var logs []modelig.IntegrationInvocationLog
	require.NoError(t, env.DB.Where("route_uuid = ?", route.RouteID).Find(&logs).Error)
	require.NotEmpty(t, logs)
}
