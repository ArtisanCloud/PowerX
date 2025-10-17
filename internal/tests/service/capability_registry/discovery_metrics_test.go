package capabilityregistry

import (
	"context"
	"testing"
	"time"

	discoveryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
)

func TestDiscoveryMetricsSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	inst := capabilityRegistryDomain.NewInstrumentation(nil)
	metrics := discoveryService.NewObservabilityMetrics(
		inst,
		discoveryService.WithHitRateThreshold(0.75),
		discoveryService.WithMinSamples(5),
		discoveryService.WithTTLEstimate(2*time.Minute),
	)

	for i := 0; i < 6; i++ {
		metrics.ObserveSync(ctx, "tenant-a", "capabilities.text.translate", discoveryService.SnapshotSourceRegistry, 2*time.Minute, nil)
	}
	metrics.ObserveSync(ctx, "tenant-a", "capabilities.text.translate", discoveryService.SnapshotSourceRegistry, 90*time.Second, nil)
	metrics.ObserveSync(ctx, "tenant-a", "capabilities.text.translate", discoveryService.SnapshotSourceRegistry, 0, context.DeadlineExceeded)

	// 3 命中、2 未命中，用于触发命中率计算。
	for i := 0; i < 3; i++ {
		metrics.ObserveCacheLookup(ctx, "tenant-a", "capabilities.text.translate", "hit")
	}
	for i := 0; i < 2; i++ {
		metrics.ObserveCacheLookup(ctx, "tenant-a", "capabilities.text.translate", "miss-refresh")
	}

	snapshot := metrics.Snapshot()
	if snapshot.CacheHits != 3 {
		t.Fatalf("expected 3 hits, got %d", snapshot.CacheHits)
	}
	if snapshot.CacheMisses != 2 {
		t.Fatalf("expected 2 misses, got %d", snapshot.CacheMisses)
	}
	expectedRate := 3.0 / 5.0
	if diff := snapshot.HitRate - expectedRate; diff > 0.0001 || diff < -0.0001 {
		t.Fatalf("expected hit rate %.2f, got %.4f", expectedRate, snapshot.HitRate)
	}
	if snapshot.SyncSuccess != 7 {
		t.Fatalf("expected 7 sync success, got %d", snapshot.SyncSuccess)
	}
	if snapshot.SyncFailures != 1 {
		t.Fatalf("expected 1 sync failure, got %d", snapshot.SyncFailures)
	}
}
