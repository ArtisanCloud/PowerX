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

const (
	shareOwnerTenantUUID = "d6a60fe9-1c3a-4bb2-96fc-4c4dee3b1172"
	shareTenantUUIDA     = "fa79c189-4b2f-4a4d-8dc0-2bc3844c2f3a"
	shareTenantUUIDB     = "6c481313-4fab-467d-8a76-7d4b2c4fce04"
	shareTenantUUIDC     = "fbc65c18-caa3-4a3c-8bd4-815bcea90b83"
)

type shareHTTPResponse struct {
	Code int `json:"code"`
	Data struct {
		ID         string `json:"id"`
		AgentID    string `json:"agent_id"`
		TenantUUID string `json:"tenant_uuid"`
		Status     string `json:"status"`
		Quotas     []struct {
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
	agentID := env.SeedAgent(shareOwnerTenantUUID, "http-share-agent")

	reqBody := map[string]any{
		"tenant_uuid":  shareTenantUUIDA,
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
	req.Header.Set("Content-Type", "application/json")
	applyTenantHeaders(req, shareOwnerTenantUUID)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	require.Equal(t, http.StatusCreated, resp.Code)

	var success shareHTTPResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &success))
	require.Equal(t, http.StatusCreated, success.Code)
	require.Equal(t, shareTenantUUIDA, success.Data.TenantUUID)
	require.Equal(t, agentID.String(), success.Data.AgentID)
	require.Equal(t, "active", success.Data.Status)
	require.Len(t, success.Data.Quotas, 1)
	require.Equal(t, "rpm", success.Data.Quotas[0].Type)
	require.Equal(t, int32(500), success.Data.Quotas[0].Limit)
	require.Equal(t, "ap-sg", success.Data.Metadata["region"])

	// duplicate share should be rejected with conflict
	dupReq := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	dupReq.Header.Set("Content-Type", "application/json")
	applyTenantHeaders(dupReq, shareOwnerTenantUUID)
	dupResp := httptest.NewRecorder()
	engine.ServeHTTP(dupResp, dupReq)
	require.Equal(t, http.StatusConflict, dupResp.Code)

	// share validator failure should surface as bad request
	env.ShareValidator.Err = fmt.Errorf("tenant not on whitelist")
	validatorBody := map[string]any{
		"tenant_uuid":  shareTenantUUIDB,
		"requested_by": "ops-admin",
		"trace_id":     "trace-http-2",
	}
	raw, _ := json.Marshal(validatorBody)
	reqInvalid := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	reqInvalid.Header.Set("Content-Type", "application/json")
	applyTenantHeaders(reqInvalid, shareOwnerTenantUUID)
	invalidResp := httptest.NewRecorder()
	engine.ServeHTTP(invalidResp, reqInvalid)
	require.Equal(t, http.StatusBadRequest, invalidResp.Code)
}

func TestRevokeAgentShareHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()
	agentID := env.SeedAgent(shareOwnerTenantUUID, "http-share-2")
	share, err := env.Deps.AgentLifecycle.Service.ShareAgent(
		context.Background(),
		agent_lifecycle.ShareInput{
			AgentID:     agentID,
			TenantUUID:  shareTenantUUIDC,
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
	req.Header.Set("Content-Type", "application/json")
	applyTenantHeaders(req, shareOwnerTenantUUID)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var success shareHTTPResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &success))
	require.Equal(t, "revoked", success.Data.Status)
	require.Equal(t, share.ID.String(), success.Data.ID)

	// revoking non-existing share should yield 404
	missingReq := httptest.NewRequest(http.MethodPost, "/api/admin/agents/shares/"+uuid.NewString()+"/revoke", bytes.NewReader(raw))
	missingReq.Header.Set("Content-Type", "application/json")
	applyTenantHeaders(missingReq, shareOwnerTenantUUID)
	missingResp := httptest.NewRecorder()
	engine.ServeHTTP(missingResp, missingReq)
	require.Equal(t, http.StatusNotFound, missingResp.Code)
}
