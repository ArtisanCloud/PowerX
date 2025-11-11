package model_routing

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/stretchr/testify/require"
)

type fakeAlert struct {
	events []SafeModeAlert
}

func (f *fakeAlert) PublishSafeModeAlert(_ context.Context, alert SafeModeAlert) {
	f.events = append(f.events, alert)
}

func TestSafeModeMonitorEnableDisable(t *testing.T) {
	cacheStore := cache.NewMemoryCache()
	svc := &Service{
		cache: cacheStore,
		inst:  instrumentation.NewInstrumentation(nil, nil),
		clock: time.Now,
	}
	alerts := &fakeAlert{}
	monitor := NewSafeModeMonitor(svc, cacheStore, SafeModeMonitorOptions{
		MinHitRate:           0.9,
		MaxFallbackRate:      0.2,
		RecoveryHitRate:      0.93,
		RecoveryFallbackRate: 0.1,
		MinDecisions:         5,
	}, alerts)
	ctx := context.Background()

	// degrade snapshot triggers safe-mode enable
	require.NoError(t, monitor.Evaluate(ctx, TelemetrySnapshot{
		Env:           "default",
		TenantScope:   "tenant-auto",
		HitRate:       0.5,
		FallbackRate:  0.3,
		DecisionCount: 200,
	}))
	state, err := svc.SafeModeState(ctx, "default", "tenant-auto")
	require.NoError(t, err)
	require.True(t, state.Enabled)
	require.Len(t, alerts.events, 1)
	require.True(t, alerts.events[0].Enabled)

	// healthy snapshot should disable
	require.NoError(t, monitor.Evaluate(ctx, TelemetrySnapshot{
		Env:           "default",
		TenantScope:   "tenant-auto",
		HitRate:       0.98,
		FallbackRate:  0.02,
		DecisionCount: 200,
	}))
	state, err = svc.SafeModeState(ctx, "default", "tenant-auto")
	require.NoError(t, err)
	require.False(t, state.Enabled)
	require.Len(t, alerts.events, 2)
	require.False(t, alerts.events[1].Enabled)
}

func TestSafeModeMonitorCooldown(t *testing.T) {
	start := time.Now()
	current := start
	cacheStore := cache.NewMemoryCache()
	svc := &Service{
		cache: cacheStore,
		inst:  instrumentation.NewInstrumentation(nil, nil),
		clock: func() time.Time { return current },
	}
	alerts := &fakeAlert{}
	monitor := NewSafeModeMonitor(svc, cacheStore, SafeModeMonitorOptions{
		MinHitRate:      0.9,
		MaxFallbackRate: 0.2,
		MinDecisions:    5,
		Cooldown:        time.Minute,
	}, alerts)
	ctx := context.Background()

	// First evaluation enables safe-mode.
	require.NoError(t, monitor.Evaluate(ctx, TelemetrySnapshot{
		Env:           "default",
		TenantScope:   "tenant-cooldown",
		HitRate:       0.4,
		FallbackRate:  0.4,
		DecisionCount: 100,
	}))
	require.True(t, alerts.events[0].Enabled)

	// Advance only 10s, still in cooldown - should not emit new alert.
	current = start.Add(10 * time.Second)
	require.NoError(t, monitor.Evaluate(ctx, TelemetrySnapshot{
		Env:           "default",
		TenantScope:   "tenant-cooldown",
		HitRate:       0.35,
		FallbackRate:  0.45,
		DecisionCount: 100,
	}))
	require.Len(t, alerts.events, 1)
}
