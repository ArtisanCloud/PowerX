//go:build ignore

package agentlifecyclecontract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type shareHTTPResponse struct {
	Code int `json:"code"`
	Data struct {
		ID       string `json:"id"`
		AgentID  string `json:"agent_id"`
		TenantID string `json:"tenant_id"`
		Status   string `json:"status"`
		Quotas   []struct {
			Type  string `json:"type"`
			Limit int32  `json:"limit"`
		} `json:"quotas"`
		Metadata map[string]string `json:"metadata"`
	} `json:"data"`
}

func TestShareAgentHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()
	agentID := env.SeedAgent("tenant-http-share", "http-share-agent")

	reqBody := map[string]any{
		"tenant_id":    "tenant-target-a",
		"requested_by": "ops-admin",
		"trace_id":     "trace-http-1",
		"quotas": []map[string]any{
			{"type": "rpm", "limit": 500},
		},
		"metadata": map[string]string{
			"region": "ap-sg",
		},
	}
	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("/api/admin/agents/%s/shares", agentID)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	require.Equal(t, http.StatusCreated, resp.Code)

	var success shareHTTPResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &success))
	require.Equal(t, http.StatusCreated, success.Code)
	require.Equal(t, "tenant-target-a", success.Data.TenantID)
	require.Equal(t, agentID.String(), success.Data.AgentID)
	require.Equal(t, "active", success.Data.Status)
	require.Len(t, success.Data.Quotas, 1)
	require.Equal(t, "rpm", success.Data.Quotas[0].Type)
	require.Equal(t, int32(500), success.Data.Quotas[0].Limit)
	require.Equal(t, "ap-sg", success.Data.Metadata["region"])

	// duplicate share should be rejected with conflict
	dupReq := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	dupReq.Header.Set("Authorization", "Bearer token")
	dupReq.Header.Set("Content-Type", "application/json")
	dupResp := httptest.NewRecorder()
	engine.ServeHTTP(dupResp, dupReq)
	require.Equal(t, http.StatusConflict, dupResp.Code)

	// share validator failure should surface as bad request
	env.ShareValidator.Err = fmt.Errorf("tenant not on whitelist")
	validatorBody := map[string]any{
		"tenant_id":    "tenant-target-b",
		"requested_by": "ops-admin",
		"trace_id":     "trace-http-2",
	}
	raw, _ := json.Marshal(validatorBody)
	reqInvalid := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	reqInvalid.Header.Set("Authorization", "Bearer token")
	reqInvalid.Header.Set("Content-Type", "application/json")
	invalidResp := httptest.NewRecorder()
	engine.ServeHTTP(invalidResp, reqInvalid)
	require.Equal(t, http.StatusBadRequest, invalidResp.Code)
}

func TestRevokeAgentShareHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()
	agentID := env.SeedAgent("tenant-http-share", "http-share-2")
	share, err := env.Deps.AgentLifecycle.Service.ShareAgent(
		context.Background(),
		agent_lifecycle.ShareInput{
			AgentID:     agentID,
			TenantID:    "tenant-target-c",
			RequestedBy: "ops-admin",
		},
	)
	require.NoError(t, err)

	body := map[string]string{
		"reason":       "cleanup",
		"requested_by": "ops-admin",
	}
	raw, _ := json.Marshal(body)
	url := fmt.Sprintf("/api/admin/agents/shares/%s/revoke", share.ID.String())
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var success shareHTTPResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &success))
	require.Equal(t, "revoked", success.Data.Status)
	require.Equal(t, share.ID.String(), success.Data.ID)

	// revoking non-existing share should yield 404
	missingReq := httptest.NewRequest(http.MethodPost, "/api/admin/agents/shares/"+uuid.NewString()+"/revoke", bytes.NewReader(raw))
	missingReq.Header.Set("Authorization", "Bearer token")
	missingReq.Header.Set("Content-Type", "application/json")
	missingResp := httptest.NewRecorder()
	engine.ServeHTTP(missingResp, missingReq)
	require.Equal(t, http.StatusNotFound, missingResp.Code)
}
