package capabilityregistry

import (
	"context"
	"testing"
	"time"

	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
)

func TestRouterMetricsSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	inst := capabilityRegistryDomain.NewInstrumentation(nil)
	metrics := router.NewRouterMetrics(inst)

	metrics.ObserveInvocation(ctx, "invoke", "capabilities.text.translate", "tenant-corex", "adapter-primary", "grpc", 120*time.Millisecond, false, nil)
	metrics.ObserveInvocation(ctx, "invoke", "capabilities.text.translate", "tenant-corex", "adapter-primary", "grpc", 640*time.Millisecond, true, nil)
	metrics.ObserveInvocation(ctx, "invoke", "capabilities.text.translate", "tenant-corex", "adapter-backup", "http", 80*time.Millisecond, false, context.DeadlineExceeded)
	metrics.ObserveFallback(ctx, "capabilities.text.translate", "tenant-corex", "primary-unhealthy")
	metrics.ObserveHealthReport(ctx, "capabilities.text.translate", "tenant-corex", "adapter-primary", "unhealthy", nil)
	metrics.ObserveHealthReport(ctx, "capabilities.text.translate", "tenant-corex", "adapter-backup", "healthy", nil)

	snapshot := metrics.Snapshot()
	if snapshot.Invocations != 3 {
		t.Fatalf("expected 3 invocations, got %d", snapshot.Invocations)
	}
	if snapshot.FallbackInvocations != 1 {
		t.Fatalf("expected 1 fallback invocation, got %d", snapshot.FallbackInvocations)
	}
	if snapshot.ErrorCount != 1 {
		t.Fatalf("expected 1 error, got %d", snapshot.ErrorCount)
	}
	if snapshot.HealthReports != 2 {
		t.Fatalf("expected 2 health reports, got %d", snapshot.HealthReports)
	}
	if snapshot.UnhealthyReports != 1 {
		t.Fatalf("expected 1 unhealthy report, got %d", snapshot.UnhealthyReports)
	}
	if snapshot.MaxFallbackLatency < 640*time.Millisecond {
		t.Fatalf("expected fallback latency >=640ms, got %s", snapshot.MaxFallbackLatency)
	}
	if snapshot.MaxLatency < 640*time.Millisecond {
		t.Fatalf("expected max latency >=640ms, got %s", snapshot.MaxLatency)
	}
}
