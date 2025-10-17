package capabilityregistry

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	capabilityRegistryHealth "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/health"
	capabilityRegistryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/gorm"
)

func TestRouterStrategyFallbackAndSticky(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registryRepo := newMockRegistryRepository()
	service := router.NewService(router.ServiceOptions{
		RegistryRepository: registryRepo,
		HealthRepository:   capabilityRegistryHealth.NewMemoryRepository(),
		EventBus:           event_bus.NewLocalEventBus(),
		Clock: func() time.Time {
			return time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
		},
	})

	// Initial invocation chooses primary adapter
	result, err := service.Invoke(ctx, router.InvokeRequest{
		CapabilityID: "capabilities.text.translate",
		TenantID:     "tenant-corex",
	})
	assertNoError(t, err)
	assertEqual(t, "adapter-primary", result.AdapterID, "initial selection")

	// Sticky key should keep adapter stable
	resultSticky, err := service.Invoke(ctx, router.InvokeRequest{
		CapabilityID: "capabilities.text.translate",
		TenantID:     "tenant-corex",
		StickyKey:    "user-1",
	})
	assertNoError(t, err)
	assertEqual(t, "adapter-primary", resultSticky.AdapterID, "sticky selection")

	// mark primary unhealthy, expect backup
	err = service.ReportHealth(ctx, router.ReportHealthInput{
		CapabilityID: "capabilities.text.translate",
		TenantID:     "tenant-corex",
		AdapterID:    "adapter-primary",
		Status:       "unhealthy",
		Reason:       "timeout",
		Failures:     3,
	})
	assertNoError(t, err)

	resultAfterFailure, err := service.Invoke(ctx, router.InvokeRequest{
		CapabilityID: "capabilities.text.translate",
		TenantID:     "tenant-corex",
	})
	assertNoError(t, err)
	assertEqual(t, "adapter-backup", resultAfterFailure.AdapterID, "fallback adapter")

	// mark backup unhealthy to trigger static fallback
	err = service.ReportHealth(ctx, router.ReportHealthInput{
		CapabilityID: "capabilities.text.translate",
		TenantID:     "tenant-corex",
		AdapterID:    "adapter-backup",
		Status:       "unhealthy",
		Reason:       "circuit-open",
		Failures:     2,
	})
	assertNoError(t, err)

	fallbackResult, err := service.Invoke(ctx, router.InvokeRequest{
		CapabilityID: "capabilities.text.translate",
		TenantID:     "tenant-corex",
	})
	assertNoError(t, err)
	assertBoolTrue(t, fallbackResult.FallbackUsed, "fallback triggered")
	assertEqual(t, "", fallbackResult.AdapterID, "no adapter when fallback")
	assertContains(t, string(fallbackResult.Payload), "fallback-static", "static fallback payload")
}

func assertContains(t *testing.T, content, substr, field string) {
	t.Helper()
	if !strings.Contains(content, substr) {
		t.Fatalf("expected %s to contain %q, got %q", field, substr, content)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertEqual[T comparable](t *testing.T, expected, actual T, field string) {
	t.Helper()
	if expected != actual {
		t.Fatalf("expected %s=%v, got %v", field, expected, actual)
	}
}

func assertBoolTrue(t *testing.T, cond bool, field string) {
	t.Helper()
	if !cond {
		t.Fatalf("expected %s to be true", field)
	}
}
