package agentlifecycleintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
)

func TestRegistrationActivationFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	events := make(chan map[string]any, 1)
	env.Bus.Subscribe("agent.lifecycle.activated", func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			events <- payload
		}
		return nil
	})

	httpEngine := env.Engine()

	registerBody := map[string]any{
		"tenant_id":                  "tenant-001",
		"alias":                      "flow-agent",
		"telemetry_contract_version": "otel-agent-v1",
	}
	registerBytes, _ := json.Marshal(registerBody)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/agent/lifecycle/agents", bytes.NewReader(registerBytes))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	httpEngine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var registerResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &registerResp))
	agentID := registerResp.Data.ID
	require.NotEmpty(t, agentID)

	activateBody := map[string]any{"tenant_id": "tenant-001"}
	activateBytes, _ := json.Marshal(activateBody)
	actReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/activate", agentID), bytes.NewReader(activateBytes))
	actReq.Header.Set("Authorization", "Bearer token")
	actReq.Header.Set("Content-Type", "application/json")
	actResp := httptest.NewRecorder()
	httpEngine.ServeHTTP(actResp, actReq)
	require.Equal(t, http.StatusOK, actResp.Code)

	select {
	case envelope := <-events:
		payload, ok := envelope["payload"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, agentID, payload["agent_id"].(string))
	case <-time.After(2 * time.Second):
		t.Fatalf("expected lifecycle activated event")
	}
}
