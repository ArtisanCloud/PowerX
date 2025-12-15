package capabilityregistry

import (
	"context"
	"sync"
	"testing"
	"time"

	discoveryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistryRouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/ArtisanCloud/PowerX/internal/tests/capability_registry/testutil"
)

func TestCrossRegionSnapshotReplication(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 11, 0, 0, 0, time.UTC)
	primaryClock := testutil.NewManualClock(baseTime)
	secondaryClock := testutil.NewManualClock(baseTime)

	registration := capabilityRegistryRouter.Registration{
		CapabilityID: "capabilities.chat.summarize",
		TenantUUID:   "tenant-enterprise",
		Status:       "published",
		Version:      12,
		Adapters: []capabilityRegistryRouter.AdapterEndpoint{
			{
				AdapterID:     "chat-grpc-primary",
				TransportType: "grpc",
				Endpoint:      "grpc://chat-primary.svc:443",
				Weight:        100,
				TimeoutMS:     5000,
			},
		},
		RoutingPolicy: capabilityRegistryRouter.RoutingPolicy{
			Strategy:         "priority",
			FallbackSequence: []string{"chat-grpc-primary"},
			CooldownSeconds:  60,
		},
	}

	primaryRepo := testutil.NewMockRegistryRepository([]capabilityRegistryRouter.Registration{registration})
	secondaryRepo := testutil.NewMockRegistryRepository(nil) // 远端依赖复制结果

	primaryCache := testutil.NewInMemoryDiscoveryCache()
	secondaryCache := testutil.NewInMemoryDiscoveryCache()

	secondaryService := discoveryService.NewService(discoveryService.ServiceOptions{
		RegistryRepository: secondaryRepo,
		CacheStore:         secondaryCache,
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
		DefaultTTL:         2 * time.Minute,
		Clock:              secondaryClock.Now,
	})

	replicator := &mockReplicator{target: secondaryService}

	primaryService := discoveryService.NewService(discoveryService.ServiceOptions{
		RegistryRepository: primaryRepo,
		CacheStore:         primaryCache,
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
		DefaultTTL:         2 * time.Minute,
		Clock:              primaryClock.Now,
		Replicator:         replicator,
	})

	_, err := primaryService.Sync(ctx, discoveryService.SyncRequest{
		TenantUUID:   "tenant-enterprise",
		Capabilities: []string{"capabilities.chat.summarize"},
		ClientID:     "edge-client",
		Force:        true,
	})
	if err != nil {
		t.Fatalf("primary sync failed: %v", err)
	}

	if replicator.replicated == 0 {
		t.Fatalf("expected replicate to remote cluster")
	}

	secondaryClock.Advance(30 * time.Second)
	replicaSnapshot, err := secondaryService.GetSnapshot(ctx, "tenant-enterprise", "capabilities.chat.summarize", "edge-client")
	if err != nil {
		t.Fatalf("secondary get snapshot failed: %v", err)
	}
	if replicaSnapshot.Source != discoveryService.SnapshotSourceReplica {
		t.Fatalf("expected replica source, got %s", replicaSnapshot.Source)
	}
	if replicaSnapshot.CapabilityID != "capabilities.chat.summarize" {
		t.Fatalf("unexpected replicated capability %s", replicaSnapshot.CapabilityID)
	}
}

type mockReplicator struct {
	mu         sync.Mutex
	target     *discoveryService.Service
	replicated int
}

func (m *mockReplicator) Replicate(ctx context.Context, snapshots []discoveryService.Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, snapshot := range snapshots {
		if err := m.target.ApplyReplica(ctx, snapshot); err != nil {
			return err
		}
		m.replicated++
	}
	return nil
}
