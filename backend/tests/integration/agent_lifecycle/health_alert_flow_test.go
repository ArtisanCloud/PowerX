package agentlifecycleintegration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
)

func TestHealthAlertFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()
	svc := env.Deps.AgentLifecycle.Service

	res, err := svc.Register(ctx, agent_lifecycle.RegisterInput{
		TenantID:                 "tenant-004",
		Alias:                    "alert-agent",
		TelemetryContractVersion: "otel-agent-v1",
	})
	require.NoError(t, err)
	agentID := res.Agent.ID

	_, err = svc.UpdateSubscription(ctx, agent_lifecycle.SubscriptionUpdateInput{
		AgentID:     agentID,
		TenantID:    "tenant-004",
		RequestedBy: "sre-robot",
		Config: agent_lifecycle.SubscriptionConfig{
			MetricsFilter:  []string{"error_rate", "p95_latency_ms"},
			HealthStatuses: []string{"degraded"},
		},
	})
	require.NoError(t, err)

	ch := make(chan map[string]any, 1)
	env.Bus.Subscribe("agent.health.degraded", func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			ch <- payload
		}
		return nil
	})

	require.NoError(t, svc.RecordHealthSnapshot(ctx, agent_lifecycle.HealthInput{
		AgentID:        agentID,
		TenantID:       "tenant-004",
		WindowDuration: time.Minute,
		Status:         "degraded",
		Metrics: agent_lifecycle.HealthMetricsInput{
			ThroughputPerMin: 40,
			SuccessRate:      0.5,
			P95LatencyMs:     2000,
			ResourceUtilPct:  0.9,
			ErrorRate:        0.45,
			AnomalyTraceIDs:  []string{"trace-alert"},
		},
	}))

	select {
	case payload := <-ch:
		require.Equal(t, res.Agent.ID.String(), payload["agent_id"].(string))
		require.Equal(t, "degraded", payload["status"])
	case <-time.After(2 * time.Second):
		t.Fatalf("expected degraded health event")
	}

	if msg, ok := env.Notifier.WaitForMessage(2 * time.Second); ok {
		require.Equal(t, "critical", msg.Severity)
		require.Contains(t, strings.ToLower(msg.Content), "alert-agent")
		require.Equal(t, agentID.String(), msg.Metadata["agent_id"])
	} else {
		t.Fatalf("expected IM notification for degraded health")
	}
}
