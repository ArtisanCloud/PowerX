package knowledge_space_contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTriggerIngestionHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	tplID := env.SeedPolicyTemplate("http-contract", "v1")
	space := env.CreateSpaceFixture("http-ingest", tplID)
	engine := env.Engine()

	t.Run("accepts ingestion job", func(t *testing.T) {
		body := map[string]any{
			"sourceType":     "pdf",
			"sourceUri":      "s3://bucket/handbook.pdf",
			"priority":       "high",
			"maskingProfile": "masking.v1",
		}
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/ingestion-jobs", space.UUID), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusAccepted, resp.Code)

		var apiResp struct {
			Data struct {
				JobID      string `json:"jobId"`
				Status     string `json:"status"`
				ChunkTotal int    `json:"chunkTotal"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.NotEmpty(t, apiResp.Data.JobID)
		require.Equal(t, "completed", apiResp.Data.Status)
		require.Greater(t, apiResp.Data.ChunkTotal, 0)
	})

	t.Run("returns 404 for unknown space", func(t *testing.T) {
		body := map[string]any{
			"sourceType": "pdf",
			"sourceUri":  "s3://bucket/ghost.pdf",
		}
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/ingestion-jobs", uuid.New()), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusNotFound, resp.Code)
	})
}
