//go:build ignore

package agentlifecycleintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const reactBridgeTenantUUID = "c266a9a7-6a9e-47c8-9cf0-118da2ed2e6e"

func TestReactTaskBridgeFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()
	svc := env.Deps.AgentLifecycle.Service
	engine := env.Engine()

	reg, err := svc.Register(ctx, agent_lifecycle.RegisterInput{
		TenantUUID:               reactBridgeTenantUUID,
		Alias:                    "bridge-agent",
		TelemetryContractVersion: "otel-agent-v1",
	})
	require.NoError(t, err)
	agentID := reg.Agent.ID

	_, err = svc.Activate(ctx, agent_lifecycle.ActivateInput{
		AgentID:     agentID,
		TenantUUID:  reactBridgeTenantUUID,
		RequestedBy: "planner",
	})
	require.NoError(t, err)

	require.NoError(t, svc.RecordHealthSnapshot(ctx, agent_lifecycle.HealthInput{
		AgentID:        agentID,
		TenantUUID:     reactBridgeTenantUUID,
		WindowDuration: time.Minute,
		Status:         "healthy",
		Metrics: agent_lifecycle.HealthMetricsInput{
			ThroughputPerMin: 40,
			SuccessRate:      0.98,
			P95LatencyMs:     900,
			ResourceUtilPct:  0.6,
			ErrorRate:        0.01,
		},
	}))

	stateReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/openapi/agents/%s/bridge/state", agentID), nil)
	stateReq.Header.Set("Authorization", "Bearer token")
	stateResp := httptest.NewRecorder()
	engine.ServeHTTP(stateResp, stateReq)
	require.Equal(t, http.StatusOK, stateResp.Code)

	var stateBody struct {
		Code int `json:"code"`
		Data struct {
			Agent struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"agent"`
			Events []struct {
				Type string `json:"type"`
			} `json:"events"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(stateResp.Body.Bytes(), &stateBody))
	require.Equal(t, agentID.String(), stateBody.Data.Agent.ID)
	require.NotEmpty(t, stateBody.Data.Events)

	lifecycleCh := make(chan map[string]any, 8)
	env.Bus.Subscribe("statebus.agent.lifecycle", captureStateBusEvent(lifecycleCh))

	freezeBody := map[string]any{
		"tenant_uuid":  reactBridgeTenantUUID,
		"reason":       "react freeze",
		"requested_by": "react-coordinator",
	}
	doBridgeRequest(t, engine, agentID, "/bridge/freeze", freezeBody)
	awaitStateBusEvent(t, lifecycleCh, "agent.lifecycle.paused")

	recoverBody := map[string]any{
		"tenant_uuid":  reactBridgeTenantUUID,
		"reason":       "react resume",
		"requested_by": "react-coordinator",
	}
	doBridgeRequest(t, engine, agentID, "/bridge/recover", recoverBody)
	awaitStateBusEvent(t, lifecycleCh, "agent.lifecycle.resumed")

	rebalanceBody := map[string]any{
		"tenant_uuid":               reactBridgeTenantUUID,
		"target_capacity_instances": 5,
		"reason":                    "rebalance",
	}
	doBridgeRequest(t, engine, agentID, "/bridge/rebalance", rebalanceBody)
	awaitStateBusEvent(t, lifecycleCh, "agent.lifecycle.scaled")
}

func doBridgeRequest(t *testing.T, engine *gin.Engine, agentID uuid.UUID, path string, body map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/openapi/agents/%s%s", agentID, path), bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
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
