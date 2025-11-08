package capabilityregistry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistryHealth "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/health"
	routerService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	sandboxService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/sandbox"
	"github.com/ArtisanCloud/PowerX/internal/tests/capability_registry/testutil"
	capabilityRegistryHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

func TestRouterSandboxHTTPInvoke(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
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
				StaticResponse: &routerService.StaticResponse{
					Payload: map[string]interface{}{
						"message": "fallback-static",
					},
					TTLSeconds: 60,
				},
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
	handler := capabilityRegistryHTTP.NewSandboxHandler(sandboxSvc)

	r := gin.New()
	r.POST("/sandbox", handler.Invoke)

	body := map[string]any{
		"capability_id": "capabilities.text.translate",
		"tenant_id":     "tenant-corex",
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/sandbox", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["adapter_id"] != "adapter-primary" {
		t.Fatalf("expected primary adapter, got %v", payload["adapter_id"])
	}
}
