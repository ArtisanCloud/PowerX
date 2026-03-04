package knowledge_space_contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
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
			"format":         "pdf",
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
				RetryCount int    `json:"retryCount"`
				ErrorCode  string `json:"errorCode"`
				ChunkTotal int    `json:"chunkTotal"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.NotEmpty(t, apiResp.Data.JobID)
		require.Equal(t, "completed", apiResp.Data.Status)
		require.Equal(t, 0, apiResp.Data.RetryCount)
		require.Empty(t, apiResp.Data.ErrorCode)
		require.Greater(t, apiResp.Data.ChunkTotal, 0)
	})

	t.Run("retries vector upsert once", func(t *testing.T) {
		env.VectorStore.SetUpsertFailures(1)
		body := map[string]any{
			"format":    "markdown",
			"sourceUri": "s3://bucket/retry.md",
		}
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/ingestion-jobs", space.UUID), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusAccepted, resp.Code)

		var apiResp struct {
			Data struct {
				Status     string `json:"status"`
				RetryCount int    `json:"retryCount"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Equal(t, "completed", apiResp.Data.Status)
		require.Equal(t, 1, apiResp.Data.RetryCount)
	})

	t.Run("blocks when masking strict required", func(t *testing.T) {
		body := map[string]any{
			"format":         "pdf",
			"sourceUri":      "s3://bucket/unmaskable.pdf",
			"maskingProfile": "masking.strict_required",
		}
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/ingestion-jobs", space.UUID), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusAccepted, resp.Code)

		var apiResp struct {
			Data struct {
				Status    string `json:"status"`
				ErrorCode string `json:"errorCode"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Equal(t, "blocked", apiResp.Data.Status)
		require.Equal(t, "masking_required", apiResp.Data.ErrorCode)
	})

	t.Run("degrades when OCR needed but not required", func(t *testing.T) {
		body := map[string]any{
			"format":      "pdf",
			"sourceUri":   "s3://bucket/scan.pdf",
			"ocrRequired": false,
		}
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/ingestion-jobs", space.UUID), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusAccepted, resp.Code)

		var apiResp struct {
			Data struct {
				Status           string  `json:"status"`
				ErrorCode        string  `json:"errorCode"`
				ChunkCoveragePct float64 `json:"chunkCoveragePct"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Equal(t, "completed", apiResp.Data.Status)
		require.Equal(t, "degraded", apiResp.Data.ErrorCode)
		require.Less(t, apiResp.Data.ChunkCoveragePct, 95.0)
	})

	t.Run("blocks when OCR required but unavailable", func(t *testing.T) {
		body := map[string]any{
			"format":      "pdf",
			"sourceUri":   "s3://bucket/scan-required.pdf",
			"ocrRequired": true,
		}
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/ingestion-jobs", space.UUID), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusAccepted, resp.Code)

		var apiResp struct {
			Data struct {
				Status    string `json:"status"`
				ErrorCode string `json:"errorCode"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Equal(t, "blocked", apiResp.Data.Status)
		require.Equal(t, "ocr_failed", apiResp.Data.ErrorCode)
	})

	t.Run("returns 404 for unknown space", func(t *testing.T) {
		body := map[string]any{
			"format":    "pdf",
			"sourceUri": "s3://bucket/ghost.pdf",
		}
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/ingestion-jobs", uuid.New()), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("rejects when embedding profile missing", func(t *testing.T) {
		noEmbedSpace := env.CreateSpaceFixture("http-ingest-no-embed", tplID)
		require.NoError(t, env.ClearSpaceEmbedding(noEmbedSpace.UUID))
		require.NoError(t, env.ClearTenantEmbeddingConfig())
		prevCfg := agentcfg.GetGlobalAIConfig()
		agentcfg.SetGlobalAIConfig(&agentcfg.AIConfig{})
		t.Cleanup(func() {
			if prevCfg != nil {
				agentcfg.SetGlobalAIConfig(prevCfg)
			}
		})
		body := map[string]any{
			"format":    "pdf",
			"sourceUri": "s3://bucket/no-embed.pdf",
		}
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/ingestion-jobs", noEmbedSpace.UUID), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
		require.Equal(t, http.StatusPreconditionFailed, resp.Code)

		var apiResp struct {
			ErrorCode string `json:"error_code"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.Equal(t, "embedding_not_configured", apiResp.ErrorCode)
	})
}
