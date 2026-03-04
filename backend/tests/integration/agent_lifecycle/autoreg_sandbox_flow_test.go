//go:build ignore

package agentlifecycleintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
)

func TestAutoRegisterSandboxFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()

	tenantUUID := "tenant-int"
	manifest := map[string]any{
		"plugin_id":                  "plugins.integration.analytics",
		"plugin_version":             "2.0.0",
		"manifest_version":           "2025-04-01",
		"alias":                      "int-agent",
		"telemetry_contract_version": "otel-agent-v1",
		"signature":                  "valid-signature",
		"default_capacity_instances": 1,
	}
	payload, _ := json.Marshal(manifest)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/agent/lifecycle/autoreg/manifests", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("tenant-uuid", tenantUUID)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var registerResp struct {
		Data struct {
			Agent struct {
				ID string `json:"id"`
			} `json:"agent"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &registerResp))
	require.NotEmpty(t, registerResp.Data.Agent.ID)

	input := env.SandboxRunner.LastInput()
	require.NotNil(t, input)
	require.Equal(t, "plugins.integration.analytics", input.PluginID)

	runBody, _ := json.Marshal(map[string]string{
		"profile": "smoke",
	})
	runReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/sandbox", registerResp.Data.Agent.ID), bytes.NewReader(runBody))
	runReq.Header.Set("Authorization", "Bearer token")
	runReq.Header.Set("Content-Type", "application/json")
	runReq.Header.Set("tenant-uuid", tenantUUID)
	runResp := httptest.NewRecorder()
	engine.ServeHTTP(runResp, runReq)
	require.Equal(t, http.StatusOK, runResp.Code)
}
