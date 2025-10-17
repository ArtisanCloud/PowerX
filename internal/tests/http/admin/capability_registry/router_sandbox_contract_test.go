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
	capabilityRegistryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	routerService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	sandboxService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/sandbox"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

func TestRouterSandboxHTTPInvoke(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	registryRepo := newMockRegistryRepository()
	routerSvc := routerService.NewService(routerService.ServiceOptions{
		RegistryRepository: registryRepo,
		HealthRepository:   capabilityRegistryHealth.NewMemoryRepository(),
		EventBus:           event_bus.NewLocalEventBus(),
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
		Clock: func() time.Time {
			return time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
		},
	})
	sandboxSvc := sandboxService.NewService(registryRepo, routerSvc)
	handler := NewSandboxHandler(sandboxSvc)

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

