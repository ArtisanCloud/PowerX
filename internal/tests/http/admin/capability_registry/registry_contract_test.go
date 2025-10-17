package capabilityregistry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	capabilityRegistryHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/gorm"
)

func TestRegistryAdminRESTContracts(t *testing.T) {
	t.Parallel()

	env := newRegistryHTTPTestEnv(t)
	t.Cleanup(env.Close)

	createResp, etag := env.createCapability(t)
	assertEqual(t, "capabilities.text.translate", createResp.CapabilityID, "capability id")
	assertEqual(t, "tenant-corex", createResp.TenantID, "tenant id")
	assertUint64(t, 1, createResp.Version, "version")
	assertInt(t, http.StatusCreated, createResp.StatusCode, "status code")
	if etag == "" {
		t.Fatal("expected ETag header")
	}

	snapshot := env.getCapability(t, "")
	assertEqual(t, createResp.CapabilityID, snapshot.CapabilityID, "snapshot capability id")
	assertUint64(t, 1, snapshot.Version, "snapshot version")
	assertEqual(t, "weighted_round_robin", snapshot.RoutingPolicy.Strategy, "routing strategy")
	assertInt(t, 2, len(snapshot.Adapters), "adapter count")

	updateResp, newETag := env.updateCapability(t, etag)
	assertUint64(t, 2, updateResp.Version, "updated version")
	assertEqual(t, "capabilities.text.translate", updateResp.CapabilityID, "updated capability id")
	assertEqual(t, "tenant-corex", updateResp.TenantID, "updated tenant id")
	if etag == newETag {
		t.Fatal("etag should change after update")
	}

	env.expectConflictOnStaleUpdate(t, etag)
	env.disableCapability(t, newETag)
	env.expectEventPublished(t, "capability.registry.updated", 3)
	env.expectNotFound(t)
}

type registryHTTPTestEnv struct {
	t          *testing.T
	server     *httptest.Server
	eventBus   *recordingEventBus
	repository *memoryRepository
	service    *capabilityRegistryService.Service
}

func newRegistryHTTPTestEnv(t *testing.T) *registryHTTPTestEnv {
	gin.SetMode(gin.TestMode)

	eventBus := newRecordingEventBus()
	repo := newMemoryRepository()
	service := capabilityRegistryService.NewService(capabilityRegistryService.ServiceOptions{
		Repository:          repo,
		EventBus:            eventBus,
		Instrumentation:     capabilityRegistryDomain.NewInstrumentation(nil),
		ContractVerifier:    alwaysPassContractVerifier{},
		ToolGrantVerifier:   alwaysPassToolGrantVerifier{},
		Clock:               func() time.Time { return time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC) },
		VersionGenerator:    capabilityRegistryService.SequenceVersionGenerator(),
		SystemActorResolver: func(context.Context) string { return "test-user" },
	})

	handler := capabilityRegistryHTTP.NewAdminHandler(capabilityRegistryHTTP.AdminHandlerOptions{Service: service})
	router := gin.New()
	group := router.Group("/admin")
	handler.Register(group)

	server := httptest.NewServer(router)

	return &registryHTTPTestEnv{
		t:          t,
		server:     server,
		eventBus:   eventBus,
		repository: repo,
		service:    service,
	}
}

func (env *registryHTTPTestEnv) Close() {
	env.server.Close()
}

func (env *registryHTTPTestEnv) createCapability(t *testing.T) (registryCreateResponse, string) {
	reqBody := map[string]interface{}{
		"capability_id":  "capabilities.text.translate",
		"tenant_id":      "tenant-corex",
		"contract_ref":   "contracts.text.translate@1.0.0",
		"status":         "published",
		"tool_grant_ids": []string{"grant-text-translate"},
		"adapters": []map[string]interface{}{
			{
				"adapter_id":     "adapter-grpc-1",
				"transport_type": "grpc",
				"service_ref":    "grpc://translator.corex.svc:443",
				"weight":         80,
				"timeout_ms":     4000,
				"labels":         map[string]string{"region": "ap-sg"},
			},
			{
				"adapter_id":     "adapter-http-2",
				"transport_type": "http",
				"endpoint_url":   "https://translator.corex/api",
				"weight":         20,
				"timeout_ms":     2500,
			},
		},
		"routing_policy": map[string]interface{}{
			"strategy":          "weighted_round_robin",
			"cooldown_seconds":  60,
			"fallback_sequence": []string{"adapter-http-2"},
		},
	}
	resp := env.doRequest(t, http.MethodPost, "/admin/capabilities", reqBody, nil)
	defer resp.Body.Close()
	assertStatus(t, http.StatusCreated, resp.StatusCode)

	var result registryCreateResponse
	decodeJSON(t, resp.Body, &result)
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("expected etag header")
	}
	result.StatusCode = resp.StatusCode
	return result, etag
}

func (env *registryHTTPTestEnv) getCapability(t *testing.T, version string) capabilitySnapshot {
	path := "/admin/capabilities/capabilities.text.translate/tenants/tenant-corex"
	if version != "" {
		path = fmt.Sprintf("%s?version=%s", path, version)
	}
	resp := env.doRequest(t, http.MethodGet, path, nil, nil)
	defer resp.Body.Close()
	assertStatus(t, http.StatusOK, resp.StatusCode)

	var snapshot capabilitySnapshot
	decodeJSON(t, resp.Body, &snapshot)
	return snapshot
}

func (env *registryHTTPTestEnv) updateCapability(t *testing.T, etag string) (registryCreateResponse, string) {
	reqBody := map[string]interface{}{
		"capability_id":  "capabilities.text.translate",
		"tenant_id":      "tenant-corex",
		"contract_ref":   "contracts.text.translate@1.0.0",
		"status":         "published",
		"tool_grant_ids": []string{"grant-text-translate"},
		"version":        1,
		"adapters": []map[string]interface{}{
			{
				"adapter_id":     "adapter-grpc-1",
				"transport_type": "grpc",
				"service_ref":    "grpc://translator.corex.svc:443",
				"weight":         60,
				"timeout_ms":     4000,
				"labels":         map[string]string{"region": "ap-sg"},
			},
			{
				"adapter_id":     "adapter-http-2",
				"transport_type": "http",
				"endpoint_url":   "https://translator.corex/api",
				"weight":         40,
				"timeout_ms":     2500,
			},
		},
		"routing_policy": map[string]interface{}{
			"strategy":          "weighted_round_robin",
			"cooldown_seconds":  60,
			"fallback_sequence": []string{"adapter-http-2"},
		},
	}
	headers := map[string]string{"If-Match": etag}
	resp := env.doRequest(t, http.MethodPut, "/admin/capabilities/capabilities.text.translate/tenants/tenant-corex", reqBody, headers)
	defer resp.Body.Close()
	assertStatus(t, http.StatusOK, resp.StatusCode)

	var result registryCreateResponse
	decodeJSON(t, resp.Body, &result)
	result.StatusCode = resp.StatusCode
	return result, resp.Header.Get("ETag")
}

func (env *registryHTTPTestEnv) expectConflictOnStaleUpdate(t *testing.T, staleETag string) {
	reqBody := map[string]interface{}{
		"capability_id": "capabilities.text.translate",
		"tenant_id":     "tenant-corex",
		"contract_ref":  "contracts.text.translate@1.0.0",
		"status":        "published",
		"version":       1,
		"adapters": []map[string]interface{}{
			{
				"adapter_id":     "adapter-grpc-1",
				"transport_type": "grpc",
				"service_ref":    "grpc://translator.corex.svc:443",
				"weight":         50,
				"timeout_ms":     4000,
			},
		},
		"routing_policy": map[string]interface{}{
			"strategy":          "weighted_round_robin",
			"cooldown_seconds":  60,
			"fallback_sequence": []string{"adapter-grpc-1"},
		},
	}
	headers := map[string]string{"If-Match": staleETag}
	resp := env.doRequest(t, http.MethodPut, "/admin/capabilities/capabilities.text.translate/tenants/tenant-corex", reqBody, headers)
	defer resp.Body.Close()
	assertStatus(t, http.StatusPreconditionFailed, resp.StatusCode)

	var apiErr apiError
	decodeJSON(t, resp.Body, &apiErr)
	assertEqual(t, "registry.version_conflict", apiErr.Code, "error code")
}

func (env *registryHTTPTestEnv) disableCapability(t *testing.T, etag string) {
	reqBody := map[string]interface{}{"reason": "deprecated capability"}
	headers := map[string]string{"If-Match": etag}
	resp := env.doRequest(t, http.MethodDelete, "/admin/capabilities/capabilities.text.translate/tenants/tenant-corex", reqBody, headers)
	defer resp.Body.Close()
	assertStatus(t, http.StatusAccepted, resp.StatusCode)
}

func (env *registryHTTPTestEnv) expectNotFound(t *testing.T) {
	resp := env.doRequest(t, http.MethodGet, "/admin/capabilities/capabilities.text.translate/tenants/tenant-corex", nil, nil)
	defer resp.Body.Close()
	assertStatus(t, http.StatusNotFound, resp.StatusCode)
}

func (env *registryHTTPTestEnv) expectEventPublished(t *testing.T, eventType string, minCount int) {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if env.eventBus.Count(eventType) >= minCount {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected at least %d events for %s", minCount, eventType)
}

func (env *registryHTTPTestEnv) doRequest(t *testing.T, method, path string, body interface{}, headers map[string]string) *http.Response {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, env.server.URL+path, reader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor-ID", "test-user")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http call: %v", err)
	}
	return resp
}

// --- Assertion helpers ---

func assertEqual[T comparable](t *testing.T, expected, actual T, field string) {
	if expected != actual {
		t.Fatalf("expected %s=%v, got %v", field, expected, actual)
	}
}

func assertInt(t *testing.T, expected, actual int, field string) {
	if expected != actual {
		t.Fatalf("expected %s=%d, got %d", field, expected, actual)
	}
}

func assertUint64(t *testing.T, expected, actual uint64, field string) {
	if expected != actual {
		t.Fatalf("expected %s=%d, got %d", field, expected, actual)
	}
}

func assertStatus(t *testing.T, expected, actual int) {
	if expected != actual {
		t.Fatalf("expected status %d, got %d", expected, actual)
	}
}

func decodeJSON(t *testing.T, reader io.Reader, dest interface{}) {
	if err := json.NewDecoder(reader).Decode(dest); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}

// --- DTOs ---

type registryCreateResponse struct {
	StatusCode   int    `json:"-"`
	CapabilityID string `json:"capability_id"`
	TenantID     string `json:"tenant_id"`
	Version      uint64 `json:"version"`
	Status       string `json:"status"`
}

type capabilitySnapshot struct {
	CapabilityID  string                `json:"capability_id"`
	TenantID      string                `json:"tenant_id"`
	ContractRef   string                `json:"contract_ref"`
	Status        string                `json:"status"`
	Version       uint64                `json:"version"`
	Adapters      []adapterSnapshot     `json:"adapters"`
	RoutingPolicy routingPolicySnapshot `json:"routing_policy"`
}

type adapterSnapshot struct {
	AdapterID     string            `json:"adapter_id"`
	TransportType string            `json:"transport_type"`
	Weight        int               `json:"weight"`
	TimeoutMS     int               `json:"timeout_ms"`
	Labels        map[string]string `json:"labels"`
}

type routingPolicySnapshot struct {
	Strategy         string   `json:"strategy"`
	FallbackSequence []string `json:"fallback_sequence"`
	CooldownSeconds  int      `json:"cooldown_seconds"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ----- Stubbed dependencies -----

type recordingEventBus struct {
	mu     sync.RWMutex
	events map[string]int
}

func newRecordingEventBus() *recordingEventBus {
	return &recordingEventBus{events: make(map[string]int)}
}

func (b *recordingEventBus) Subscribe(string, event_bus.Handler) func() { return func() {} }

func (b *recordingEventBus) Publish(eventType string, _ interface{}, _ context.Context) {
	b.mu.Lock()
	b.events[eventType]++
	b.mu.Unlock()
}

func (b *recordingEventBus) Close() error { return nil }

func (b *recordingEventBus) Count(eventType string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.events[eventType]
}

type alwaysPassContractVerifier struct{}

func (alwaysPassContractVerifier) VerifyContract(context.Context, string, string) error { return nil }

type alwaysPassToolGrantVerifier struct{}

func (alwaysPassToolGrantVerifier) VerifyToolGrants(context.Context, string, []string) error {
	return nil
}

// ----- In-memory repository for tests -----

type memoryRepository struct {
	mu            sync.RWMutex
	registrations map[string][]capabilityRegistryService.Registration
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{registrations: make(map[string][]capabilityRegistryService.Registration)}
}

func (r *memoryRepository) Create(ctx context.Context, _ *gorm.DB, reg capabilityRegistryService.Registration) (capabilityRegistryService.Registration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := repoKey(reg.CapabilityID, reg.TenantID)
	r.registrations[key] = append(r.registrations[key], reg)
	return reg, nil
}

func (r *memoryRepository) Update(ctx context.Context, _ *gorm.DB, reg capabilityRegistryService.Registration, expectedVersion uint64) (capabilityRegistryService.Registration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := repoKey(reg.CapabilityID, reg.TenantID)
	list := r.registrations[key]
	if len(list) == 0 {
		return capabilityRegistryService.Registration{}, capabilityRegistryService.ErrRegistrationNotFound
	}
	current := list[len(list)-1]
	if current.Version != expectedVersion {
		return capabilityRegistryService.Registration{}, capabilityRegistryService.ErrVersionConflict
	}
	r.registrations[key] = append(list, reg)
	return reg, nil
}

func (r *memoryRepository) GetLatest(ctx context.Context, _ *gorm.DB, capabilityID, tenantID string) (capabilityRegistryService.Registration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := r.registrations[repoKey(capabilityID, tenantID)]
	if len(list) == 0 {
		return capabilityRegistryService.Registration{}, capabilityRegistryService.ErrRegistrationNotFound
	}
	return list[len(list)-1], nil
}

func (r *memoryRepository) GetVersion(ctx context.Context, _ *gorm.DB, capabilityID, tenantID string, version uint64) (capabilityRegistryService.Registration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, reg := range r.registrations[repoKey(capabilityID, tenantID)] {
		if reg.Version == version {
			return reg, nil
		}
	}
	return capabilityRegistryService.Registration{}, capabilityRegistryService.ErrRegistrationNotFound
}

func (r *memoryRepository) Disable(ctx context.Context, _ *gorm.DB, capabilityID, tenantID, reason, actor string, expectedVersion uint64, next capabilityRegistryService.Registration) (capabilityRegistryService.Registration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := repoKey(capabilityID, tenantID)
	list := r.registrations[key]
	if len(list) == 0 {
		return capabilityRegistryService.Registration{}, capabilityRegistryService.ErrRegistrationNotFound
	}
	current := list[len(list)-1]
	if expectedVersion > 0 && current.Version != expectedVersion {
		return capabilityRegistryService.Registration{}, capabilityRegistryService.ErrVersionConflict
	}
	r.registrations[key] = append(list, next)
	return next, nil
}

func repoKey(capabilityID, tenantID string) string { return capabilityID + "::" + tenantID }

func (r *memoryRepository) ListLatest(ctx context.Context, _ *gorm.DB, tenantID string, limit, offset int) ([]capabilityRegistryService.Registration, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []capabilityRegistryService.Registration
	for key, regs := range r.registrations {
		parts := strings.Split(key, "::")
		if len(parts) != 2 || parts[1] != tenantID {
			continue
		}
		if len(regs) == 0 {
			continue
		}
		all = append(all, regs[len(regs)-1])
	}
	total := int64(len(all))
	if limit <= 0 {
		return all, total, nil
	}
	start := offset
	if start > len(all) {
		start = len(all)
	}
	end := start + limit
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}
