package knowledge_space_contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
)

func TestTriggerIngestionHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()
	body := map[string]any{
		"sourceType": "pdf",
		"sourceUri":  "s3://bucket/handbook.pdf",
		"priority":   "high",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/ingestion-jobs", env.TenantID()), bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusAccepted, resp.Code)

	var apiResp struct {
		Data struct {
			JobID string `json:"jobId"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
	require.NotEmpty(t, apiResp.Data.JobID)
	require.Equal(t, "completed", apiResp.Data.Status)
}
