//go:build ignore

package agentlifecyclecontract

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

func TestTenantAgentFormHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()

	formBody := map[string]any{
		"tenant_id":                  "tenant-form",
		"alias":                      "marketing-agent",
		"display_name":               "Marketing Agent",
		"telemetry_contract_version": "otel-agent-v1",
		"purpose":                    "handle marketing requests",
		"permissions":                []string{"crm.read"},
		"tool_grants": []map[string]string{
			{"name": "crm", "version": "v1"},
		},
		"requested_by": "tenant-admin",
	}
	payload, _ := json.Marshal(formBody)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/agents/tenant/forms", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var submitResp struct {
		Code int `json:"code"`
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &submitResp))
	require.Equal(t, http.StatusCreated, submitResp.Code)
	require.Equal(t, "pending_approval", submitResp.Data.Status)

	formID := submitResp.Data.ID

	approveBody := map[string]string{"operator": "ops-user"}
	approveBytes, _ := json.Marshal(approveBody)
	approveReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agents/tenant/forms/%s/approve", formID), bytes.NewReader(approveBytes))
	approveReq.Header.Set("Authorization", "Bearer token")
	approveReq.Header.Set("Content-Type", "application/json")
	approveResp := httptest.NewRecorder()
	engine.ServeHTTP(approveResp, approveReq)
	require.Equal(t, http.StatusOK, approveResp.Code)

	var approveData struct {
		Code int `json:"code"`
		Data struct {
			Status           string `json:"status"`
			ActivatedAgentID string `json:"activated_agent_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(approveResp.Body.Bytes(), &approveData))
	require.Equal(t, "activated", approveData.Data.Status)
	require.NotEmpty(t, approveData.Data.ActivatedAgentID)

	// conflict alias
	conflictBody := formBody
	conflictBody["alias"] = "root-admin"
	conflictBytes, _ := json.Marshal(conflictBody)
	conflictReq := httptest.NewRequest(http.MethodPost, "/api/admin/agents/tenant/forms", bytes.NewReader(conflictBytes))
	conflictReq.Header.Set("Authorization", "Bearer token")
	conflictReq.Header.Set("Content-Type", "application/json")
	conflictResp := httptest.NewRecorder()
	engine.ServeHTTP(conflictResp, conflictReq)
	require.Equal(t, http.StatusBadRequest, conflictResp.Code)
}
