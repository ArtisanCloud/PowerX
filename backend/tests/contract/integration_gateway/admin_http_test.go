//go:build ignore

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

const integrationAdminTenantUUID = "4c0bc0d0-4c87-4c62-8a23-7f6ed0bc5d11"

type successResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type routePayload struct {
	RouteID        string   `json:"route_id"`
	TenantUUID     string   `json:"tenant_uuid"`
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
		"tenant_uuid":    integrationAdminTenantUUID,
		"route_slug":     "crm-sync",
		"capability_id":  "cap.crm.sync",
		"tool_grant_ids": []string{"grant-crm"},
		"channels":       []string{"http"},
	}
	bodyBytes, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/integration/routes", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp := serveIntegrationHTTPRequest(t, engine, req, "")
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
	getResp := serveIntegrationHTTPRequest(t, engine, getReq, "")
	require.Equal(t, http.StatusOK, getResp.Code)

	var getData struct {
		Code int          `json:"code"`
		Data routePayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &getData))
	require.Equal(t, createResp.Data.RouteID, getData.Data.RouteID)

	// list routes
	listReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/integration/routes?tenant_uuid=%s", integrationAdminTenantUUID), nil)
	listResp := serveIntegrationHTTPRequest(t, engine, listReq, "")
	require.Equal(t, http.StatusOK, listResp.Code)

	var listData listResponse
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listData))
	require.GreaterOrEqual(t, len(listData.Data.Items), 1)

	// update route
	updateBody := map[string]any{
		"tenant_uuid":  integrationAdminTenantUUID,
		"channels":     []string{"http", "mcp"},
		"description":  "updated description",
		"event_topics": map[string]string{"updated": "integration.gateway.route.updated"},
	}
	updateBytes, _ := json.Marshal(updateBody)
	updateReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/integration/routes/%s", createResp.Data.RouteID), bytes.NewReader(updateBytes))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("If-Match", etag)
	updateResp := serveIntegrationHTTPRequest(t, engine, updateReq, "")
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
	suspendBody, _ := json.Marshal(map[string]string{"tenant_uuid": integrationAdminTenantUUID, "reason": "maintenance"})
	suspendReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/integration/routes/%s/suspend", createResp.Data.RouteID), bytes.NewReader(suspendBody))
	suspendReq.Header.Set("Content-Type", "application/json")
	suspendResp := serveIntegrationHTTPRequest(t, engine, suspendReq, "")
	require.Equal(t, http.StatusOK, suspendResp.Code)

	var suspendData struct {
		Code int          `json:"code"`
		Data routePayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(suspendResp.Body.Bytes(), &suspendData))
	require.Equal(t, "suspended", suspendData.Data.LifecycleState)

	// resume route
	resumeBody, _ := json.Marshal(map[string]string{"tenant_uuid": integrationAdminTenantUUID})
	resumeReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/integration/routes/%s/resume", createResp.Data.RouteID), bytes.NewBuffer(resumeBody))
	resumeReq.Header.Set("Content-Type", "application/json")
	resumeResp := serveIntegrationHTTPRequest(t, engine, resumeReq, "")
	require.Equal(t, http.StatusOK, resumeResp.Code)

	var resumeData struct {
		Code int          `json:"code"`
		Data routePayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resumeResp.Body.Bytes(), &resumeData))
	require.Equal(t, "active", resumeData.Data.LifecycleState)

	// retire route
	retireBody, _ := json.Marshal(map[string]string{"tenant_uuid": integrationAdminTenantUUID})
	retireReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/integration/routes/%s/retire", createResp.Data.RouteID), bytes.NewBuffer(retireBody))
	retireReq.Header.Set("Content-Type", "application/json")
	retireResp := serveIntegrationHTTPRequest(t, engine, retireReq, "")
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
	versionResp := serveIntegrationHTTPRequest(t, engine, versionReq, "")
	require.Equal(t, http.StatusOK, versionResp.Code)

	var versions versionsResponse
	require.NoError(t, json.Unmarshal(versionResp.Body.Bytes(), &versions))
	require.GreaterOrEqual(t, len(versions.Data.Items), 3)
}
