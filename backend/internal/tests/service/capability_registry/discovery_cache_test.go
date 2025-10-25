package capabilityregistry

import (
	"context"
	"errors"
	"testing"
	"time"

	discoveryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistryRouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/ArtisanCloud/PowerX/internal/tests/capability_registry/testutil"
)

func TestDiscoveryServiceCacheAndFallback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseTime := time.Date(2025, 2, 3, 9, 30, 0, 0, time.UTC)
	clock := testutil.NewManualClock(baseTime)

	registryRepo := testutil.NewMockRegistryRepository([]capabilityRegistryRouter.Registration{
		{
			CapabilityID: "capabilities.text.translate",
			TenantID:     "tenant-corex",
			Status:       "published",
			Version:      7,
			Adapters: []capabilityRegistryRouter.AdapterEndpoint{
				{
					AdapterID:     "adapter-primary",
					TransportType: "grpc",
					Endpoint:      "grpc://translator.corex.svc:443",
					Weight:        70,
					TimeoutMS:     3500,
				},
				{
					AdapterID:     "adapter-backup",
					TransportType: "http",
					Endpoint:      "https://translator.corex/api",
					Weight:        30,
					TimeoutMS:     4000,
				},
			},
			RoutingPolicy: capabilityRegistryRouter.RoutingPolicy{
				Strategy:         "weighted_round_robin",
				CooldownSeconds:  60,
				FallbackSequence: []string{"adapter-backup"},
			},
			FallbackPlan: &capabilityRegistryRouter.FallbackPlan{
				FallbackTargets: []string{"capabilities.text.translate.backup"},
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

	// 首次同步
	snapshots, err := service.Sync(ctx, discoveryService.SyncRequest{
		TenantID:     "tenant-corex",
		Capabilities: []string{"capabilities.text.translate"},
		ClientID:     "sdk",
		Force:        true,
	})
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	snapshot := snapshots[0]
	if snapshot.CapabilityID != "capabilities.text.translate" {
		t.Fatalf("unexpected capability id %s", snapshot.CapabilityID)
	}
	if snapshot.ExpiresAt.Sub(snapshot.IssuedAt) != 2*time.Minute {
		t.Fatalf("expected ttl 2m, got %v", snapshot.ExpiresAt.Sub(snapshot.IssuedAt))
	}

	// TTL 内读取
	clock.Advance(90 * time.Second)
	cached, err := service.GetSnapshot(ctx, "tenant-corex", "capabilities.text.translate", "sdk")
	if err != nil {
		t.Fatalf("get snapshot failed: %v", err)
	}
	if cached.Stale {
		t.Fatalf("expected fresh snapshot within ttl")
	}
	if cached.MetadataDigest == "" {
		t.Fatalf("expected metadata digest populated")
	}

	// 模拟 Registry 不可用并推进至过期
	registryRepo.SetError(errors.New("registry down"))
	clock.Advance(2 * time.Minute)
	stale, err := service.GetSnapshot(ctx, "tenant-corex", "capabilities.text.translate", "sdk")
	if err != nil {
		t.Fatalf("expect stale snapshot fallback, got error %v", err)
	}
	if !stale.Stale {
		t.Fatalf("expected stale snapshot flag when registry unavailable")
	}
	if stale.Source != discoveryService.SnapshotSourceCache {
		t.Fatalf("expected cache source for stale snapshot, got %s", stale.Source)
	}

	// 恢复 Registry，执行强制刷新
	registryRepo.SetError(nil)
	clock.Advance(30 * time.Second)
	refreshed, err := service.Sync(ctx, discoveryService.SyncRequest{
		TenantID:     "tenant-corex",
		Capabilities: []string{"capabilities.text.translate"},
		ClientID:     "sdk",
		Force:        true,
	})
	if err != nil {
		t.Fatalf("force sync failed: %v", err)
	}
	if len(refreshed) != 1 {
		t.Fatalf("expected 1 refreshed snapshot, got %d", len(refreshed))
	}
	if !refreshed[0].IssuedAt.After(stale.IssuedAt) {
		t.Fatalf("expected refreshed snapshot issued time newer")
	}
}
