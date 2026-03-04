package capabilityregistry

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	capabilityRegistryGRPC "github.com/ArtisanCloud/PowerX/internal/transport/grpc/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/gorm"
)

const bufSize = 1024 * 1024

func TestRegistryGRPCContracts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tenantCtx := capabilityRegistryContext(t, ctx, defaultTenantUUID)
	env := newRegistryGRPCTestEnv(t)
	t.Cleanup(env.Close)

	createResp, err := env.client.CreateCapability(tenantCtx, &capabilityRegistryPB.CreateCapabilityRequest{
		Registration: &capabilityRegistryPB.CapabilityRegistration{
			Id:           &capabilityRegistryPB.TenantScopedId{CapabilityId: "capabilities.text.translate", TenantUuid: defaultTenantUUID},
			ContractRef:  "contracts.text.translate@1.0.0",
			Status:       "published",
			ToolGrantIds: []string{"grant-text-translate"},
			Adapters: []*capabilityRegistryPB.AdapterEndpoint{
				{AdapterId: "adapter-grpc-1", TransportType: "grpc", Endpoint: "grpc://translator.corex.svc:443", Weight: 80, TimeoutMs: 4000, Labels: map[string]string{"region": "ap-sg"}},
				{AdapterId: "adapter-http-2", TransportType: "http", Endpoint: "https://translator.corex/api", Weight: 20, TimeoutMs: 2500},
			},
			RoutingPolicy: &capabilityRegistryPB.RoutingPolicy{Strategy: "weighted_round_robin", CooldownSeconds: 60, FallbackSequence: []string{"adapter-http-2"}},
		},
	})
	assertNoError(t, err)
	assertNotNil(t, createResp, "create response")
	assertUint64(t, 1, createResp.GetRegistration().GetVersion(), "create version")
	assertNoCapabilityRegistryTenantLeak(t, createResp)

	getResp, err := env.client.GetCapability(tenantCtx, &capabilityRegistryPB.GetCapabilityRequest{
		Id: &capabilityRegistryPB.TenantScopedId{CapabilityId: "capabilities.text.translate", TenantUuid: defaultTenantUUID},
	})
	assertNoError(t, err)
	assertUint64(t, 1, getResp.GetRegistration().GetVersion(), "get version")
	assertInt(t, 2, len(getResp.GetRegistration().GetAdapters()), "adapter count")
	assertNoCapabilityRegistryTenantLeak(t, getResp)

	updateResp, err := env.client.UpdateCapability(tenantCtx, &capabilityRegistryPB.UpdateCapabilityRequest{
		Registration: &capabilityRegistryPB.CapabilityRegistration{
			Id:          &capabilityRegistryPB.TenantScopedId{CapabilityId: "capabilities.text.translate", TenantUuid: defaultTenantUUID},
			ContractRef: "contracts.text.translate@1.0.0",
			Status:      "published",
			Version:     1,
			Adapters: []*capabilityRegistryPB.AdapterEndpoint{
				{AdapterId: "adapter-grpc-1", TransportType: "grpc", Endpoint: "grpc://translator.corex.svc:443", Weight: 60, TimeoutMs: 4000, Labels: map[string]string{"region": "ap-sg"}},
				{AdapterId: "adapter-http-2", TransportType: "http", Endpoint: "https://translator.corex/api", Weight: 40, TimeoutMs: 2500},
			},
			RoutingPolicy: &capabilityRegistryPB.RoutingPolicy{Strategy: "weighted_round_robin", CooldownSeconds: 60, FallbackSequence: []string{"adapter-http-2"}},
		},
	})
	assertNoError(t, err)
	assertUint64(t, 2, updateResp.GetRegistration().GetVersion(), "update version")
	assertNoCapabilityRegistryTenantLeak(t, updateResp)

	_, err = env.client.UpdateCapability(tenantCtx, &capabilityRegistryPB.UpdateCapabilityRequest{
		Registration: &capabilityRegistryPB.CapabilityRegistration{
			Id:            &capabilityRegistryPB.TenantScopedId{CapabilityId: "capabilities.text.translate", TenantUuid: defaultTenantUUID},
			ContractRef:   "contracts.text.translate@1.0.0",
			Status:        "published",
			Version:       1,
			Adapters:      []*capabilityRegistryPB.AdapterEndpoint{{AdapterId: "adapter-grpc-1", TransportType: "grpc", Endpoint: "grpc://translator.corex.svc:443", Weight: 55, TimeoutMs: 4000}},
			RoutingPolicy: &capabilityRegistryPB.RoutingPolicy{Strategy: "weighted_round_robin", CooldownSeconds: 60, FallbackSequence: []string{"adapter-grpc-1"}},
		},
	})
	assertStatusCode(t, codes.FailedPrecondition, err)

	disableResp, err := env.client.DisableCapability(tenantCtx, &capabilityRegistryPB.DisableCapabilityRequest{
		Id:     &capabilityRegistryPB.TenantScopedId{CapabilityId: "capabilities.text.translate", TenantUuid: defaultTenantUUID},
		Reason: "deprecated capability",
	})
	assertNoError(t, err)
	assertUint64(t, 3, disableResp.GetRegistration().GetVersion(), "disable version")
	assertEqual(t, "disabled", disableResp.GetRegistration().GetStatus(), "disable status")
	assertNoCapabilityRegistryTenantLeak(t, disableResp)

	_, err = env.client.GetCapability(tenantCtx, &capabilityRegistryPB.GetCapabilityRequest{
		Id: &capabilityRegistryPB.TenantScopedId{CapabilityId: "capabilities.text.translate", TenantUuid: defaultTenantUUID},
	})
	assertStatusCode(t, codes.NotFound, err)

	env.expectEventCount(t, "capability.registry.updated", 3)
}

type registryGRPCTestEnv struct {
	listener *bufconn.Listener
	server   *grpc.Server
	client   capabilityRegistryPB.CapabilityRegistryServiceClient
	eventBus *recordingEventBusHandlers
	repo     *memoryRepository
}

func newRegistryGRPCTestEnv(t *testing.T) *registryGRPCTestEnv {
	listener := bufconn.Listen(bufSize)
	server := grpc.NewServer()

	eventBus := newRecordingEventBusHandlers()
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

	capabilityRegistryGRPC.RegisterCapabilityRegistryServer(server, service)

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Logf("gRPC server stopped: %v", err)
		}
	}()

	conn, err := grpc.DialContext(context.Background(), "bufconn", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithInsecure(), grpc.WithBlock())
	assertNoError(t, err)

	client := capabilityRegistryPB.NewCapabilityRegistryServiceClient(conn)

	return &registryGRPCTestEnv{
		listener: listener,
		server:   server,
		client:   client,
		eventBus: eventBus,
		repo:     repo,
	}
}

func (env *registryGRPCTestEnv) Close() {
	env.server.GracefulStop()
	env.listener.Close()
}

func (env *registryGRPCTestEnv) expectEventCount(t *testing.T, event string, min int) {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if env.eventBus.Count(event) >= min {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected at least %d events for %s", min, event)
}

// --- Assertion helpers ----------------------------------------------------

func assertNoError(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertNotNil(t *testing.T, v interface{}, field string) {
	if v == nil {
		t.Fatalf("expected %s to be non-nil", field)
	}
}

func assertUint64(t *testing.T, expected, actual uint64, field string) {
	if expected != actual {
		t.Fatalf("expected %s=%d, got %d", field, expected, actual)
	}
}

func assertInt(t *testing.T, expected, actual int, field string) {
	if expected != actual {
		t.Fatalf("expected %s=%d, got %d", field, expected, actual)
	}
}

func assertEqual(t *testing.T, expected, actual string, field string) {
	if expected != actual {
		t.Fatalf("expected %s=%s, got %s", field, expected, actual)
	}
}

func assertStatusCode(t *testing.T, code codes.Code, err error) {
	if err == nil {
		t.Fatalf("expected grpc error %v", code)
	}
	statusErr, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if statusErr.Code() != code {
		t.Fatalf("expected code %v, got %v", code, statusErr.Code())
	}
}

// --- Recording event bus with handlers ------------------------------------

type recordingEventBusHandlers struct {
	mu       sync.RWMutex
	events   map[string]int
	handlers map[string][]event_bus.Handler
}

func newRecordingEventBusHandlers() *recordingEventBusHandlers {
	return &recordingEventBusHandlers{events: make(map[string]int), handlers: make(map[string][]event_bus.Handler)}
}

func (b *recordingEventBusHandlers) Subscribe(eventType string, handler event_bus.Handler) func() {
	b.mu.Lock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
	b.mu.Unlock()
	return func() {}
}

func (b *recordingEventBusHandlers) Publish(eventType string, payload interface{}, ctx context.Context) {
	b.mu.Lock()
	b.events[eventType]++
	for _, handler := range b.handlers[eventType] {
		_ = handler(event_bus.Event{Name: eventType, Payload: payload, Ctx: ctx})
	}
	b.mu.Unlock()
}

func (b *recordingEventBusHandlers) Close() error { return nil }

func (b *recordingEventBusHandlers) Count(eventType string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.events[eventType]
}

// --- In-memory repository and verifiers (same as HTTP tests) --------------

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
	key := repoKey(reg.CapabilityID, reg.TenantUUID)
	r.registrations[key] = append(r.registrations[key], reg)
	return reg, nil
}

func (r *memoryRepository) Update(ctx context.Context, _ *gorm.DB, reg capabilityRegistryService.Registration, expectedVersion uint64) (capabilityRegistryService.Registration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := repoKey(reg.CapabilityID, reg.TenantUUID)
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

func (r *memoryRepository) GetLatest(ctx context.Context, _ *gorm.DB, capabilityID, tenantUUID string) (capabilityRegistryService.Registration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := r.registrations[repoKey(capabilityID, tenantUUID)]
	if len(list) == 0 {
		return capabilityRegistryService.Registration{}, capabilityRegistryService.ErrRegistrationNotFound
	}
	return list[len(list)-1], nil
}

func (r *memoryRepository) GetVersion(ctx context.Context, _ *gorm.DB, capabilityID, tenantUUID string, version uint64) (capabilityRegistryService.Registration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, reg := range r.registrations[repoKey(capabilityID, tenantUUID)] {
		if reg.Version == version {
			return reg, nil
		}
	}
	return capabilityRegistryService.Registration{}, capabilityRegistryService.ErrRegistrationNotFound
}

func (r *memoryRepository) Disable(ctx context.Context, _ *gorm.DB, capabilityID, tenantUUID, reason, actor string, expectedVersion uint64, next capabilityRegistryService.Registration) (capabilityRegistryService.Registration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := repoKey(capabilityID, tenantUUID)
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

func (r *memoryRepository) ListLatest(ctx context.Context, _ *gorm.DB, tenantUUID string, limit, offset int) ([]capabilityRegistryService.Registration, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []capabilityRegistryService.Registration
	for key, regs := range r.registrations {
		parts := strings.Split(key, "::")
		if len(parts) != 2 || parts[1] != tenantUUID {
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

func repoKey(capabilityID, tenantUUID string) string {
	return capabilityID + "::" + tenantUUID
}

type alwaysPassContractVerifier struct{}

func (alwaysPassContractVerifier) VerifyContract(context.Context, string, string) error { return nil }

type alwaysPassToolGrantVerifier struct{}

func (alwaysPassToolGrantVerifier) VerifyToolGrants(context.Context, string, []string) error {
	return nil
}
