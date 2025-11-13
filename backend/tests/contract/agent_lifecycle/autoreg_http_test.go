//go:build ignore

package agentlifecyclecontract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
)

func TestAutoRegisterManifestHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()

	payload := map[string]any{
		"plugin_id":                  "plugins.demo.analytics",
		"plugin_version":             "1.2.3",
		"manifest_version":           "2025-03-01",
		"tenant_id":                  "tenant-auto",
		"alias":                      "analytics-agent",
		"display_name":               "Analytics Agent",
		"telemetry_contract_version": "otel-agent-v1",
		"tool_grants": []map[string]string{
			{"name": "search", "version": "v1"},
		},
		"default_capacity_instances": 2,
		"sandbox_profile":            "full",
		"signature":                  "valid-signature",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/agent/lifecycle/autoreg/manifests", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var data struct {
		Code int `json:"code"`
		Data struct {
			Agent struct {
				ID     string `json:"id"`
				Alias  string `json:"alias"`
				Status string `json:"status"`
			} `json:"agent"`
			Sandbox struct {
				Status string `json:"status"`
			} `json:"sandbox"`
			DryRun bool `json:"dry_run"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &data))
	require.Equal(t, http.StatusCreated, data.Code)
	require.Equal(t, "analytics-agent", data.Data.Agent.Alias)
	require.Equal(t, "pending", data.Data.Agent.Status)
	require.Equal(t, "completed", data.Data.Sandbox.Status)
	require.False(t, data.Data.DryRun)

	lastInput := env.SandboxRunner.LastInput()
	require.NotNil(t, lastInput)
	require.Equal(t, "plugins.demo.analytics", lastInput.PluginID)
	require.Equal(t, "full", lastInput.Profile)

	// invalid signature should be rejected
	payload["signature"] = "invalid"
	body, _ = json.Marshal(payload)
	badReq := httptest.NewRequest(http.MethodPost, "/api/admin/agent/lifecycle/autoreg/manifests", bytes.NewReader(body))
	badReq.Header.Set("Authorization", "Bearer token")
	badReq.Header.Set("Content-Type", "application/json")
	badResp := httptest.NewRecorder()
	engine.ServeHTTP(badResp, badReq)
	require.Equal(t, http.StatusUnauthorized, badResp.Code)
}
