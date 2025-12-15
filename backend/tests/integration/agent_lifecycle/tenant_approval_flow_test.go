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

const tenantApprovalTenantUUID = "0f5e41e7-21fe-4e87-9e2c-9c816ce44205"

func TestTenantApprovalFlowIntegration(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()

	activated := make(chan map[string]any, 1)
	env.Bus.Subscribe("agent.lifecycle.activated", func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			activated <- payload
		}
		return nil
	})

	body := map[string]any{
		"tenant_uuid":                tenantApprovalTenantUUID,
		"alias":                      "tenant-int-agent",
		"display_name":               "Tenant INT Agent",
		"telemetry_contract_version": "otel-agent-v1",
		"permissions":                []string{"crm.write"},
		"requested_by":               "tenant-admin",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/agents/tenant/forms", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var submitResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &submitResp))

	approveBody := map[string]string{"operator": "ops-reviewer"}
	approveBytes, _ := json.Marshal(approveBody)
	approveReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agents/tenant/forms/%s/approve", submitResp.Data.ID), bytes.NewReader(approveBytes))
	approveReq.Header.Set("Authorization", "Bearer token")
	approveReq.Header.Set("Content-Type", "application/json")
	approveResp := httptest.NewRecorder()
	engine.ServeHTTP(approveResp, approveReq)
	require.Equal(t, http.StatusOK, approveResp.Code)

	select {
	case payload := <-activated:
		require.Equal(t, "tenant-int-agent", payload["alias"])
	case <-time.After(2 * time.Second):
		t.Fatalf("expected lifecycle activated event")
	}
}
