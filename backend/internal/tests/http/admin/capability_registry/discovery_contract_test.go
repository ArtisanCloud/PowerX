//go:build ignore

package capabilityregistry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	discoveryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistryRouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/ArtisanCloud/PowerX/internal/tests/capability_registry/testutil"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry"
)

func TestDiscoveryHTTPContract(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	baseTime := time.Date(2025, 2, 3, 10, 0, 0, 0, time.UTC)
	clock := testutil.NewManualClock(baseTime)

	registryRepo := testutil.NewMockRegistryRepository([]capabilityRegistryRouter.Registration{
		{
			CapabilityID: "capabilities.text.translate",
			TenantID:     "tenant-corex",
			Status:       "published",
			Version:      3,
			Adapters: []capabilityRegistryRouter.AdapterEndpoint{
				{
					AdapterID:     "adapter-primary",
					TransportType: "grpc",
					Endpoint:      "grpc://translator.corex.svc:443",
					Weight:        80,
					TimeoutMS:     4000,
				},
			},
			RoutingPolicy: capabilityRegistryRouter.RoutingPolicy{
				Strategy:         "weighted_round_robin",
				CooldownSeconds:  60,
				FallbackSequence: []string{"adapter-primary"},
			},
			FallbackPlan: &capabilityRegistryRouter.FallbackPlan{
				FallbackTargets: []string{},
				StaticResponse: &capabilityRegistryRouter.StaticResponse{
					Payload: map[string]interface{}{
						"message": "fallback-static",
					},
					TTLSeconds: 60,
				},
			},
		},
	})

	cacheStore := testutil.NewInMemoryDiscoveryCache()

	service := discoveryService.NewService(discoveryService.ServiceOptions{
		RegistryRepository: registryRepo,
		CacheStore:         cacheStore,
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
		DefaultTTL:         2 * time.Minute,
		Clock:              clock.Now,
	})

	handler := capability_registry.NewDiscoveryHandler(service)

	router := gin.New()
	router.GET("/discovery/:tenantId/:capabilityId", handler.GetSnapshot)
	router.POST("/discovery/sync", handler.Sync)

	// 1. 尚未同步时返回 404
	req := httptest.NewRequest(http.MethodGet, "/discovery/tenant-corex/capabilities.text.translate", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 before sync, got %d", resp.Code)
	}

	// 2. 触发同步
	syncPayload := map[string]any{
		"tenant_id":    "tenant-corex",
		"capabilities": []string{"capabilities.text.translate"},
		"client_id":    "sdk-client",
	}
	body, _ := json.Marshal(syncPayload)
	syncReq := httptest.NewRequest(http.MethodPost, "/discovery/sync", bytes.NewReader(body))
	syncReq.Header.Set("Content-Type", "application/json")
	syncResp := httptest.NewRecorder()
	router.ServeHTTP(syncResp, syncReq)
	if syncResp.Code != http.StatusOK {
		t.Fatalf("expected sync 200, got %d body=%s", syncResp.Code, syncResp.Body.String())
	}
	var syncResult []map[string]any
	if err := json.Unmarshal(syncResp.Body.Bytes(), &syncResult); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if len(syncResult) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(syncResult))
	}
	if syncResult[0]["capability_id"] != "capabilities.text.translate" {
		t.Fatalf("unexpected capability id %v", syncResult[0]["capability_id"])
	}

	// 3. 再次获取缓存，校验 TTL 头与数据
	clock.Advance(30 * time.Second)
	getReq := httptest.NewRequest(http.MethodGet, "/discovery/tenant-corex/capabilities.text.translate", nil)
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected cache hit 200, got %d body=%s", getResp.Code, getResp.Body.String())
	}
	cacheControl := getResp.Header().Get("Cache-Control")
	if cacheControl == "" {
		t.Fatalf("expected Cache-Control header present")
	}
	if cacheControl != "max-age=90" && cacheControl != "max-age=120" {
		t.Fatalf("unexpected Cache-Control value %q", cacheControl)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(getResp.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snapshot["fallback_plan"] == nil {
		t.Fatalf("expected fallback plan to be present")
	}
	if snapshot["metadata_digest"] == "" {
		t.Fatalf("expected metadata digest to be populated")
	}
}
