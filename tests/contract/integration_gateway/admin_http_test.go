package integrationgatewaycontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/tests/integration_gateway/testenv"
	"github.com/stretchr/testify/require"
)

type successResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type routePayload struct {
	RouteID        string   `json:"route_id"`
	TenantID       string   `json:"tenant_id"`
	RouteSlug      string   `json:"route_slug"`
	CapabilityID   string   `json:"capability_id"`
	ToolGrantIDs   []string `json:"tool_grant_ids"`
	Channels       []string `json:"channels"`
	LifecycleState string   `json:"lifecycle_state"`
	Status         string   `json:"status"`
	CurrentVersion uint32   `json:"current_version"`
}

type listResponse struct {
	Code int `json:"code"`
	Data struct {
		Items []routePayload `json:"items"`
	} `json:"data"`
}

type versionsResponse struct {
	Code int `json:"code"`
	Data struct {
		Items []struct {
			Version    uint32 `json:"version"`
			ChangeType string `json:"change_type"`
		} `json:"items"`
	} `json:"data"`
}

func TestAdminHTTPWorkflow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()

	// create route
	createBody := map[string]any{
		"tenant_id":      "tenant-001",
		"route_slug":     "crm-sync",
		"capability_id":  "cap.crm.sync",
		"tool_grant_ids": []string{"grant-crm"},
		"channels":       []string{"http"},
	}
	bodyBytes, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/integration/routes", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)

	var createResp struct {
		Code int          `json:"code"`
		Data routePayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &createResp))
	require.Equal(t, http.StatusCreated, createResp.Code)
	require.NotEmpty(t, createResp.Data.RouteID)
	require.Equal(t, "crm-sync", createResp.Data.RouteSlug)
	require.Equal(t, "active", createResp.Data.LifecycleState)

	etag := resp.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// get route
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/integration/routes/%s", createResp.Data.RouteID), nil)
	getReq.Header.Set("Authorization", "Bearer token")
	getResp := httptest.NewRecorder()
	engine.ServeHTTP(getResp, getReq)
	require.Equal(t, http.StatusOK, getResp.Code)

	var getData struct {
		Code int          `json:"code"`
		Data routePayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &getData))
	require.Equal(t, createResp.Data.RouteID, getData.Data.RouteID)

	// list routes
	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/integration/routes?tenant_id=tenant-001", nil)
	listReq.Header.Set("Authorization", "Bearer token")
	listResp := httptest.NewRecorder()
	engine.ServeHTTP(listResp, listReq)
	require.Equal(t, http.StatusOK, listResp.Code)

	var listData listResponse
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listData))
	require.GreaterOrEqual(t, len(listData.Data.Items), 1)

	// update route
	updateBody := map[string]any{
		"tenant_id":    "tenant-001",
		"channels":     []string{"http", "mcp"},
		"description":  "updated description",
		"event_topics": map[string]string{"updated": "integration.gateway.route.updated"},
	}
	updateBytes, _ := json.Marshal(updateBody)
	updateReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/integration/routes/%s", createResp.Data.RouteID), bytes.NewReader(updateBytes))
	updateReq.Header.Set("Authorization", "Bearer token")
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("If-Match", etag)
	updateResp := httptest.NewRecorder()
	engine.ServeHTTP(updateResp, updateReq)
	require.Equal(t, http.StatusOK, updateResp.Code)

	var updateData struct {
		Code int          `json:"code"`
		Data routePayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(updateResp.Body.Bytes(), &updateData))
	require.ElementsMatch(t, []string{"http", "mcp"}, updateData.Data.Channels)
	etag = updateResp.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// suspend route
	suspendBody, _ := json.Marshal(map[string]string{"tenant_id": "tenant-001", "reason": "maintenance"})
	suspendReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/integration/routes/%s/suspend", createResp.Data.RouteID), bytes.NewReader(suspendBody))
	suspendReq.Header.Set("Authorization", "Bearer token")
	suspendReq.Header.Set("Content-Type", "application/json")
	suspendResp := httptest.NewRecorder()
	engine.ServeHTTP(suspendResp, suspendReq)
	require.Equal(t, http.StatusOK, suspendResp.Code)

	var suspendData struct {
		Code int          `json:"code"`
		Data routePayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(suspendResp.Body.Bytes(), &suspendData))
	require.Equal(t, "suspended", suspendData.Data.LifecycleState)

	// resume route
	resumeBody, _ := json.Marshal(map[string]string{"tenant_id": "tenant-001"})
	resumeReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/integration/routes/%s/resume", createResp.Data.RouteID), bytes.NewBuffer(resumeBody))
	resumeReq.Header.Set("Authorization", "Bearer token")
	resumeReq.Header.Set("Content-Type", "application/json")
	resumeResp := httptest.NewRecorder()
	engine.ServeHTTP(resumeResp, resumeReq)
	require.Equal(t, http.StatusOK, resumeResp.Code)

	var resumeData struct {
		Code int          `json:"code"`
		Data routePayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resumeResp.Body.Bytes(), &resumeData))
	require.Equal(t, "active", resumeData.Data.LifecycleState)

	// retire route
	retireBody, _ := json.Marshal(map[string]string{"tenant_id": "tenant-001"})
	retireReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/integration/routes/%s/retire", createResp.Data.RouteID), bytes.NewBuffer(retireBody))
	retireReq.Header.Set("Authorization", "Bearer token")
	retireReq.Header.Set("Content-Type", "application/json")
	retireResp := httptest.NewRecorder()
	engine.ServeHTTP(retireResp, retireReq)
	require.Equal(t, http.StatusOK, retireResp.Code)

	var retireData struct {
		Code int          `json:"code"`
		Data routePayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(retireResp.Body.Bytes(), &retireData))
	require.Equal(t, "retired", retireData.Data.LifecycleState)
	require.Equal(t, "disabled", retireData.Data.Status)

	// list versions
	versionReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/integration/routes/%s/versions", createResp.Data.RouteID), nil)
	versionReq.Header.Set("Authorization", "Bearer token")
	versionResp := httptest.NewRecorder()
	engine.ServeHTTP(versionResp, versionReq)
	require.Equal(t, http.StatusOK, versionResp.Code)

	var versions versionsResponse
	require.NoError(t, json.Unmarshal(versionResp.Body.Bytes(), &versions))
	require.GreaterOrEqual(t, len(versions.Data.Items), 3)
}
