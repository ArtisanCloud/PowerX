package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAIConfigSetsRuntimeLoopLimitDefaults(t *testing.T) {
	cfg := AIConfig{}
	cfg.SetDefaults()
	require.Equal(t, 5*time.Minute, cfg.Defaults.LLM.RequestTimeout)
	require.Equal(t, AIRuntime{MaxPlanRevisions: 2, MaxObservations: 4, MaxSteps: 16, MaxCapabilityCalls: 8, MaxConcurrentTasks: 4, RunDeadline: 30 * time.Minute, QueueWaitTimeout: 10 * time.Minute}, cfg.Runtime)
}

func TestRuntimeBudgetLimitsRejectInvalidLimits(t *testing.T) {
	SetGlobalAIConfig(&AIConfig{
		Runtime: AIRuntime{MaxPlanRevisions: 2, MaxObservations: 4, MaxSteps: 4, MaxCapabilityCalls: 5, MaxConcurrentTasks: 4},
	})
	_, err := RuntimeBudgetLimits()
	require.EqualError(t, err, "ai.runtime loop limits exceed max_steps")
}

func TestDurableTenantQuotaDefaultsAndBounds(t *testing.T) {
	cfg := DurableSessions{Enabled: true, ReportBucket: "runs", WorkerConcurrency: 4, ScanInterval: time.Second, LeaseTTL: time.Second}
	require.Equal(t, 16, cfg.WithLifecycleDefaults().TenantConcurrency)
	require.NoError(t, cfg.Validate())
	for _, limit := range []int{-1, 10001} {
		cfg.TenantConcurrency = limit
		require.ErrorContains(t, cfg.Validate(), "tenant_concurrency")
	}
	cfg.TenantConcurrency = 1
	require.NoError(t, cfg.Validate())
}
