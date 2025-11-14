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

func TestDeltaHTTPFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	tpl := env.SeedPolicyTemplate("delta-http", "v1")
	space := env.CreateSpaceFixture("delta-http-space", tpl)
	engine := env.Engine()

	var jobID string

	t.Run("start job", func(t *testing.T) {
		payload := map[string]any{
			"spaceId":    space.UUID.String(),
			"source":     "handbook",
			"packageUri": "s3://demo/delta.tar.gz",
			"notes":      "auto-test",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/delta/jobs", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		require.Equal(t, http.StatusAccepted, resp.Code)
		var apiResp struct {
			Data struct {
				JobID  string `json:"jobId"`
				Status string `json:"status"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
		require.NotEmpty(t, apiResp.Data.JobID)
		require.Equal(t, "generated", apiResp.Data.Status)
		jobID = apiResp.Data.JobID
	})

	t.Run("fetch report", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/knowledge/delta/reports/%s", jobID), nil)
		req.Header.Set("Authorization", "Bearer token")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		require.Equal(t, http.StatusOK, resp.Code)
	})

	t.Run("publish job", func(t *testing.T) {
		payload := map[string]any{
			"jobId":        jobID,
			"decision":     "publish",
			"approvedBy":   "ops@powerx.io",
			"diffAccuracy": 99.1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/delta/publish", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		require.Equal(t, http.StatusOK, resp.Code)
	})

	t.Run("rollback job", func(t *testing.T) {
		payload := map[string]any{
			"jobId":  jobID,
			"reason": "verification failed",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/version/rollback", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		require.Equal(t, http.StatusOK, resp.Code)
	})

	t.Run("unknown job returns 404", func(t *testing.T) {
		payload := map[string]any{
			"jobId":    uuid.New().String(),
			"decision": "publish",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/knowledge/delta/publish", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		require.Equal(t, http.StatusNotFound, resp.Code)
	})
}
