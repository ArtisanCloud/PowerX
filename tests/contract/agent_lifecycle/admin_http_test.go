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

	// pause agent
	pauseBody, _ := json.Marshal(map[string]string{
		"tenant_id": "tenant-001",
		"reason":    "maintenance",
	})
	pauseReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/pause", agentID), bytes.NewReader(pauseBody))
	pauseReq.Header.Set("Authorization", "Bearer token")
	pauseReq.Header.Set("Content-Type", "application/json")
	pauseResp := httptest.NewRecorder()
	engine.ServeHTTP(pauseResp, pauseReq)
	require.Equal(t, http.StatusOK, pauseResp.Code)

	var pauseData struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(pauseResp.Body.Bytes(), &pauseData))
	require.Equal(t, "paused", pauseData.Data.Status)

	// resume agent
	resumeBody, _ := json.Marshal(map[string]string{
		"tenant_id": "tenant-001",
	})
	resumeReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/resume", agentID), bytes.NewReader(resumeBody))
	resumeReq.Header.Set("Authorization", "Bearer token")
	resumeReq.Header.Set("Content-Type", "application/json")
	resumeResp := httptest.NewRecorder()
	engine.ServeHTTP(resumeResp, resumeReq)
	require.Equal(t, http.StatusOK, resumeResp.Code)

	var resumeData struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resumeResp.Body.Bytes(), &resumeData))
	require.Equal(t, "active", resumeData.Data.Status)

	// scale agent
	scaleBody, _ := json.Marshal(map[string]any{
		"tenant_id":                 "tenant-001",
		"target_capacity_instances": 5,
	})
	scaleReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/scale", agentID), bytes.NewReader(scaleBody))
	scaleReq.Header.Set("Authorization", "Bearer token")
	scaleReq.Header.Set("Content-Type", "application/json")
	scaleResp := httptest.NewRecorder()
	engine.ServeHTTP(scaleResp, scaleReq)
	require.Equal(t, http.StatusOK, scaleResp.Code)

	var scaleData struct {
		Code int `json:"code"`
		Data struct {
			Status                   string `json:"status"`
			CurrentCapacityInstances int32  `json:"current_capacity_instances"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(scaleResp.Body.Bytes(), &scaleData))
	require.Equal(t, "active", scaleData.Data.Status)
	require.Equal(t, int32(5), scaleData.Data.CurrentCapacityInstances)

	// retire agent
	retireBody, _ := json.Marshal(map[string]string{
		"tenant_id": "tenant-001",
		"reason":    "sunset",
	})
	retireReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/agent/lifecycle/agents/%s/retire", agentID), bytes.NewReader(retireBody))
	retireReq.Header.Set("Authorization", "Bearer token")
	retireReq.Header.Set("Content-Type", "application/json")
	retireResp := httptest.NewRecorder()
	engine.ServeHTTP(retireResp, retireReq)
	require.Equal(t, http.StatusOK, retireResp.Code)

	var retireData struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(retireResp.Body.Bytes(), &retireData))
	require.Equal(t, "retired", retireData.Data.Status)
}
