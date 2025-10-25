package agentlifecycle_unit

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/stretchr/testify/require"
)

func TestUpdateSubscriptionSanitizeAndPersist(t *testing.T) {
	res := newTestResources(t)
	ctx := context.Background()
	profile := res.seedProfile("active")

	cfg, err := res.service.UpdateSubscription(ctx, agent_lifecycle.SubscriptionUpdateInput{
		AgentID:     profile.UUID,
		TenantID:    profile.TenantID,
		RequestedBy: "tester",
		Config: agent_lifecycle.SubscriptionConfig{
			MetricsFilter:  []string{"ERROR_RATE", "P95_LATENCY_MS", "p95_latency_ms"},
			HealthStatuses: []string{"DEGRADED", "UnAvailable", "degraded"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"error_rate", "p95_latency_ms"}, cfg.MetricsFilter)
	require.Equal(t, []string{"degraded", "unavailable"}, cfg.HealthStatuses)

	loaded, err := res.service.GetSubscription(ctx, profile.UUID)
	require.NoError(t, err)
	require.Equal(t, cfg.HealthStatuses, loaded.HealthStatuses)
	require.Equal(t, cfg.MetricsFilter, loaded.MetricsFilter)
	require.False(t, loaded.UpdatedAt.IsZero())
}

func TestUpdateSubscriptionInvalidStatus(t *testing.T) {
	res := newTestResources(t)
	ctx := context.Background()
	profile := res.seedProfile("active")

	_, err := res.service.UpdateSubscription(ctx, agent_lifecycle.SubscriptionUpdateInput{
		AgentID:     profile.UUID,
		TenantID:    profile.TenantID,
		RequestedBy: "tester",
		Config: agent_lifecycle.SubscriptionConfig{
			MetricsFilter:  []string{"error_rate"},
			HealthStatuses: []string{},
		},
	})
	require.ErrorIs(t, err, agent_lifecycle.ErrInvalidSubscription)
}

func TestGetSubscriptionReturnsDefault(t *testing.T) {
	res := newTestResources(t)
	ctx := context.Background()
	profile := res.seedProfile("active")

	cfg, err := res.service.GetSubscription(ctx, profile.UUID)
	require.NoError(t, err)
	require.Equal(t, []string{"degraded", "unavailable"}, cfg.HealthStatuses)
	require.Equal(t, []string{"error_rate", "p95_latency_ms", "success_rate"}, cfg.MetricsFilter)
}
