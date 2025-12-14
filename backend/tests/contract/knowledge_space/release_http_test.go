package knowledge_space_contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
)

func TestReleaseHTTPFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()

	var policyID string
	var batchToken string

	t.Run("upsert policy", func(t *testing.T) {
		payload := map[string]any{
			"matrixVersion": "v2025.02",
			"pilotTenants":  []string{"demo-retail"},
			"batches": []map[string]any{
				{"name": "pilot", "tenants": []string{"demo-retail"}},
				{"name": "wave-2", "tenants": []string{"demo-lite", "demo-enterprise"}},
			},
			"guardrails": map[string]string{"latency_p95": "<5m"},
			"approvedBy": "ops@powerx.io",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/release/policies", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusCreated, resp.Code)
		var apiResp struct {
			Data struct {
				PolicyID json.Number `json:"policyId"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.NotEmpty(t, apiResp.Data.PolicyID)
		policyID = apiResp.Data.PolicyID.String()
	})

	t.Run("publish release", func(t *testing.T) {
		payload := map[string]any{
			"policyId":    policyID,
			"versionId":   "ver-2025.02",
			"requestedBy": "qa@powerx.io",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/release/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusOK, resp.Code)
		var apiResp struct {
			Data struct {
				BatchToken string   `json:"batchToken"`
				BatchIndex int      `json:"batchIndex"`
				Tenants    []string `json:"tenants"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Equal(t, 0, apiResp.Data.BatchIndex)
		require.Len(t, apiResp.Data.Tenants, 1)
		batchToken = apiResp.Data.BatchToken
		require.NotEmpty(t, batchToken)
	})

	t.Run("promote batch", func(t *testing.T) {
		payload := map[string]any{
			"policyId":    policyID,
			"versionId":   "ver-2025.02",
			"batchToken":  batchToken,
			"requestedBy": "ops@powerx.io",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/release/promote", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusOK, resp.Code)
		var apiResp struct {
			Data struct {
				BatchToken string   `json:"batchToken"`
				Tenants    []string `json:"tenants"`
				State      string   `json:"state"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Equal(t, "promoted", apiResp.Data.State)
		require.Len(t, apiResp.Data.Tenants, 2)
		batchToken = apiResp.Data.BatchToken
	})

	t.Run("rollback", func(t *testing.T) {
		payload := map[string]any{
			"policyId":    policyID,
			"versionId":   "ver-2025.02",
			"reason":      "anomaly",
			"requestedBy": "ops@powerx.io",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/release/rollback", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusOK, resp.Code)
		var apiResp struct {
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Equal(t, "rolled_back", apiResp.Data.Status)
	})
}
