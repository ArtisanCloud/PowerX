//go:build ignore

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

	activated := make(chan map[string]any, 1)
	paused := make(chan map[string]any, 1)
	resumed := make(chan map[string]any, 1)
	scaled := make(chan map[string]any, 1)
	retired := make(chan map[string]any, 1)

	env.Bus.Subscribe("agent.lifecycle.activated", func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			activated <- payload
		}
		return nil
	})
	env.Bus.Subscribe("agent.lifecycle.paused", func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			paused <- payload
		}
		return nil
	})
	env.Bus.Subscribe("agent.lifecycle.resumed", func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			resumed <- payload
		}
		return nil
	})
	env.Bus.Subscribe("agent.lifecycle.scaled", func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			scaled <- payload
		}
		return nil
	})
	env.Bus.Subscribe("agent.lifecycle.retired", func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			retired <- payload
		}
		return nil
	})

	httpEngine := env.Engine()

	tenantUUID := "tenant-001"
	registerBody := map[string]any{
		"alias":                      "flow-agent",
		"telemetry_contract_version": "otel-agent-v1",
	}
	registerBytes, _ := json.Marshal(registerBody)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/agent/lifecycle/agents", bytes.NewReader(registerBytes))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PowerX-Tenant", tenantUUID)
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

	activateBytes, _ := json.Marshal(map[string]any{})
	actReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/activate", agentID), bytes.NewReader(activateBytes))
	actReq.Header.Set("Authorization", "Bearer token")
	actReq.Header.Set("Content-Type", "application/json")
	actReq.Header.Set("X-PowerX-Tenant", tenantUUID)
	actResp := httptest.NewRecorder()
	httpEngine.ServeHTTP(actResp, actReq)
	require.Equal(t, http.StatusOK, actResp.Code)

	select {
	case payload := <-activated:
		require.Equal(t, agentID, payload["agent_id"].(string))
	case <-time.After(2 * time.Second):
		t.Fatalf("expected lifecycle activated event")
	}

	// pause
	pauseBody, _ := json.Marshal(map[string]string{})
	pauseReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/pause", agentID), bytes.NewReader(pauseBody))
	pauseReq.Header.Set("Authorization", "Bearer token")
	pauseReq.Header.Set("Content-Type", "application/json")
	pauseReq.Header.Set("X-PowerX-Tenant", tenantUUID)
	pauseResp := httptest.NewRecorder()
	httpEngine.ServeHTTP(pauseResp, pauseReq)
	require.Equal(t, http.StatusOK, pauseResp.Code)

	select {
	case payload := <-paused:
		require.Equal(t, agentID, payload["agent_id"].(string))
	case <-time.After(2 * time.Second):
		t.Fatalf("expected lifecycle paused event")
	}

	// resume
	resumeBody, _ := json.Marshal(map[string]string{})
	resumeReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/resume", agentID), bytes.NewReader(resumeBody))
	resumeReq.Header.Set("Authorization", "Bearer token")
	resumeReq.Header.Set("Content-Type", "application/json")
	resumeReq.Header.Set("X-PowerX-Tenant", tenantUUID)
	resumeResp := httptest.NewRecorder()
	httpEngine.ServeHTTP(resumeResp, resumeReq)
	require.Equal(t, http.StatusOK, resumeResp.Code)

	select {
	case payload := <-resumed:
		require.Equal(t, agentID, payload["agent_id"].(string))
	case <-time.After(2 * time.Second):
		t.Fatalf("expected lifecycle resumed event")
	}

	// scale
	scaleBody, _ := json.Marshal(map[string]any{"target_capacity_instances": 4})
	scaleReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/scale", agentID), bytes.NewReader(scaleBody))
	scaleReq.Header.Set("Authorization", "Bearer token")
	scaleReq.Header.Set("Content-Type", "application/json")
	scaleReq.Header.Set("X-PowerX-Tenant", tenantUUID)
	scaleResp := httptest.NewRecorder()
	httpEngine.ServeHTTP(scaleResp, scaleReq)
	require.Equal(t, http.StatusOK, scaleResp.Code)

	select {
	case payload := <-scaled:
		require.Equal(t, agentID, payload["agent_id"].(string))
		capVal, ok := payload["to_capacity"].(int32)
		if !ok {
			if v, ok2 := payload["to_capacity"].(int); ok2 {
				capVal = int32(v)
				ok = true
			}
		}
		require.True(t, ok)
		require.Equal(t, int32(4), capVal)
	case <-time.After(2 * time.Second):
		t.Fatalf("expected lifecycle scaled event")
	}

	// retire
	retireBody, _ := json.Marshal(map[string]string{})
	retireReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/retire", agentID), bytes.NewReader(retireBody))
	retireReq.Header.Set("Authorization", "Bearer token")
	retireReq.Header.Set("Content-Type", "application/json")
	retireReq.Header.Set("X-PowerX-Tenant", tenantUUID)
	retireResp := httptest.NewRecorder()
	httpEngine.ServeHTTP(retireResp, retireReq)
	require.Equal(t, http.StatusOK, retireResp.Code)

	select {
	case payload := <-retired:
		require.Equal(t, agentID, payload["agent_id"].(string))
	case <-time.After(2 * time.Second):
		t.Fatalf("expected lifecycle retired event")
	}
}
