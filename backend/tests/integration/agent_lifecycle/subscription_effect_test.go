//go:build ignore

package agentlifecycleintegration

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionEffect(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()
	svc := env.Deps.AgentLifecycle.Service

	reg, err := svc.Register(ctx, agent_lifecycle.RegisterInput{
		TenantID:                 "tenant-005",
		Alias:                    "subscription-effect",
		TelemetryContractVersion: "otel-agent-v1",
	})
	require.NoError(t, err)

	agentID := reg.Agent.ID

	// 初始化订阅：退化与不可用都触发告警
	_, err = svc.UpdateSubscription(ctx, agent_lifecycle.SubscriptionUpdateInput{
		AgentID:  agentID,
		TenantID: "tenant-005",
		Config: agent_lifecycle.SubscriptionConfig{
			MetricsFilter:  []string{"error_rate"},
			HealthStatuses: []string{"degraded", "unavailable"},
		},
	})
	require.NoError(t, err)

	degradedEvents := make(chan map[string]any, 2)
	unavailableEvents := make(chan map[string]any, 1)

	env.Bus.Subscribe("agent.health.degraded", func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			degradedEvents <- payload
		}
		return nil
	})
	env.Bus.Subscribe("agent.health.unavailable", func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			unavailableEvents <- payload
		}
		return nil
	})

	// 退化快照 -> 触发通知与事件
	require.NoError(t, svc.RecordHealthSnapshot(ctx, agent_lifecycle.HealthInput{
		AgentID:        agentID,
		TenantID:       "tenant-005",
		WindowDuration: time.Minute,
		Status:         "degraded",
		Metrics: agent_lifecycle.HealthMetricsInput{
			ThroughputPerMin: 55,
			SuccessRate:      0.6,
			P95LatencyMs:     2100,
			ResourceUtilPct:  0.87,
			ErrorRate:        0.42,
		},
	}))

	if msg, ok := env.Notifier.WaitForMessage(2 * time.Second); ok {
		require.Equal(t, "critical", msg.Severity)
	} else {
		t.Fatalf("expected degraded notification")
	}
	select {
	case <-degradedEvents:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected degraded event")
	}

	// 调整订阅，仅在不可用时发送通知
	env.Notifier.Reset()
	_, err = svc.UpdateSubscription(ctx, agent_lifecycle.SubscriptionUpdateInput{
		AgentID:  agentID,
		TenantID: "tenant-005",
		Config: agent_lifecycle.SubscriptionConfig{
			MetricsFilter:  []string{"success_rate"},
			HealthStatuses: []string{"unavailable"},
		},
	})
	require.NoError(t, err)

	// 再次退化 -> 仍发布事件，但不应有通知
	require.NoError(t, svc.RecordHealthSnapshot(ctx, agent_lifecycle.HealthInput{
		AgentID:        agentID,
		TenantID:       "tenant-005",
		WindowDuration: time.Minute,
		Status:         "degraded",
		Metrics: agent_lifecycle.HealthMetricsInput{
			ThroughputPerMin: 50,
			SuccessRate:      0.55,
			P95LatencyMs:     2300,
			ResourceUtilPct:  0.9,
			ErrorRate:        0.4,
		},
	}))

	select {
	case <-degradedEvents:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected degraded event after subscription change")
	}

	if _, ok := env.Notifier.WaitForMessage(1 * time.Second); ok {
		t.Fatalf("notification should be suppressed by subscription filter")
	}

	// 不可用 -> 应再次触发通知
	require.NoError(t, svc.RecordHealthSnapshot(ctx, agent_lifecycle.HealthInput{
		AgentID:        agentID,
		TenantID:       "tenant-005",
		WindowDuration: time.Minute,
		Status:         "unavailable",
		Metrics: agent_lifecycle.HealthMetricsInput{
			ThroughputPerMin: 10,
			SuccessRate:      0.2,
			P95LatencyMs:     3500,
			ResourceUtilPct:  0.95,
			ErrorRate:        0.8,
		},
	}))

	if msg, ok := env.Notifier.WaitForMessage(2 * time.Second); ok {
		require.Equal(t, "critical", msg.Severity)
		require.Equal(t, agentID.String(), msg.Metadata["agent_id"])
	} else {
		t.Fatalf("expected unavailable notification")
	}

	select {
	case <-unavailableEvents:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected unavailable event")
	}
}
