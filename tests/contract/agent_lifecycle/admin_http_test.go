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

func TestAdminHTTPRegisterAndActivate(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()

	body := map[string]any{
		"tenant_id":                  "tenant-001",
		"alias":                      "content-writer",
		"display_name":               "Content Writer",
		"telemetry_contract_version": "otel-agent-v1",
		"default_capacity_instances": 2,
		"tool_grants": []map[string]string{
			{"name": "summarize", "version": "v1"},
		},
		"metadata": map[string]string{
			"team": "marketing",
		},
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/agent/lifecycle/agents", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var registerResp struct {
		Code int `json:"code"`
		Data struct {
			ID              string `json:"id"`
			TenantID        string `json:"tenant_id"`
			Alias           string `json:"alias"`
			Status          string `json:"status"`
			EventTopic      string `json:"event_topic_prefix"`
			Telemetry       string `json:"telemetry_contract_version"`
			DefaultCapacity int32  `json:"default_capacity_instances"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &registerResp))
	require.NotEmpty(t, registerResp.Data.ID)
	require.Equal(t, http.StatusCreated, registerResp.Code)
	require.Equal(t, "pending", registerResp.Data.Status)
	require.Equal(t, "tenant-001", registerResp.Data.TenantID)

	agentID := registerResp.Data.ID

	// 获取 agent
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s", agentID), nil)
	getReq.Header.Set("Authorization", "Bearer token")
	getResp := httptest.NewRecorder()
	engine.ServeHTTP(getResp, getReq)
	require.Equal(t, http.StatusOK, getResp.Code)

	var getData struct {
		Code int `json:"code"`
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &getData))
	require.Equal(t, agentID, getData.Data.ID)
	require.Equal(t, "pending", getData.Data.Status)

	// 激活 agent
	activateBody, _ := json.Marshal(map[string]string{
		"tenant_id": "tenant-001",
		"reason":    "initial rollout",
	})
	actReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/activate", agentID), bytes.NewReader(activateBody))
	actReq.Header.Set("Authorization", "Bearer token")
	actReq.Header.Set("Content-Type", "application/json")
	actResp := httptest.NewRecorder()
	engine.ServeHTTP(actResp, actReq)
	require.Equal(t, http.StatusOK, actResp.Code)

	var actData struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(actResp.Body.Bytes(), &actData))
	require.Equal(t, "active", actData.Data.Status)
}
