//go:build ignore

package agentlifecyclecontract

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
)

func TestStateBusEventSchema(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	lifecycleEvents := make(chan map[string]any, 4)
	healthEvents := make(chan map[string]any, 4)

	env.Bus.Subscribe("statebus.agent.lifecycle", captureStateBusEvent(lifecycleEvents))
	env.Bus.Subscribe("statebus.agent.health", captureStateBusEvent(healthEvents))

	ctx := context.Background()
	svc := env.Deps.AgentLifecycle.Service

	reg, err := svc.Register(ctx, agent_lifecycle.RegisterInput{
		TenantID:                 "tenant-statebus",
		Alias:                    "statebus-agent",
		TelemetryContractVersion: "otel-agent-v1",
	})
	require.NoError(t, err)

	_, err = svc.Activate(ctx, agent_lifecycle.ActivateInput{
		AgentID:     reg.Agent.ID,
		TenantID:    "tenant-statebus",
		RequestedBy: "ops",
	})
	require.NoError(t, err)

	event := awaitStateBusEvent(t, lifecycleEvents, "agent.lifecycle.activated")
	assertHasKeys(t, event, "event", "source", "timestamp", "payload")
	payload := decodePayload(t, event["payload"])
	require.Equal(t, reg.Agent.ID.String(), payload["agent_id"])

	err = svc.RecordHealthSnapshot(ctx, agent_lifecycle.HealthInput{
		AgentID:        reg.Agent.ID,
		TenantID:       "tenant-statebus",
		WindowDuration: time.Minute,
		Status:         "degraded",
		Metrics: agent_lifecycle.HealthMetricsInput{
			ThroughputPerMin: 30,
			SuccessRate:      0.8,
			P95LatencyMs:     2200,
			ResourceUtilPct:  0.7,
			ErrorRate:        0.2,
		},
	})
	require.NoError(t, err)

	healthEvent := awaitStateBusEvent(t, healthEvents, "agent.health.degraded")
	payload = decodePayload(t, healthEvent["payload"])
	require.Equal(t, "tenant-statebus", payload["tenant_id"])
	require.Equal(t, "degraded", payload["status"])
}

func captureStateBusEvent(ch chan<- map[string]any) event_bus.Handler {
	return func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			select {
			case ch <- payload:
			default:
			}
		}
		return nil
	}
}

func awaitStateBusEvent(t *testing.T, ch <-chan map[string]any, expected string) map[string]any {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case evt := <-ch:
			if evt["event"] == expected {
				return evt
			}
		case <-timeout:
			t.Fatalf("expected event %s", expected)
		}
	}
}

func decodePayload(t *testing.T, raw any) map[string]any {
	t.Helper()
	switch val := raw.(type) {
	case map[string]any:
		return val
	case string:
		var out map[string]any
		require.NoError(t, json.Unmarshal([]byte(val), &out))
		return out
	default:
		t.Fatalf("unexpected payload type %T", raw)
		return nil
	}
}

func assertHasKeys(t *testing.T, event map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := event[key]; !ok {
			t.Fatalf("missing key %s", key)
		}
	}
}
