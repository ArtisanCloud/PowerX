package knowledge_space_contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
)

func TestQABridgeHTTPHandlers(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	policyID := env.SeedPolicyTemplate("http-qa-bridge", "v1")
	spaceA := env.CreateSpaceFixture("http-qa-space-a", policyID)
	spaceB := env.CreateSpaceFixture("http-qa-space-b", policyID)
	require.NoError(t, env.ActivateSpace(spaceA.UUID))
	require.NoError(t, env.SetSpaceStatus(spaceB.UUID, "retired"))

	engine := env.Engine()

	payload, _ := json.Marshal(map[string]any{
		"intent":          "测试检索计划",
		"domainTags":      []string{"ops"},
		"sessionId":       "qa-http-session",
		"latencyBudgetMs": 1500,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/openapi/knowledge-spaces/qa/retrieval-plan", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Less(t, time.Since(started), 2*time.Second)
	require.Equal(t, http.StatusOK, resp.Code)

	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			CandidateSpaces []struct {
				SpaceID       string `json:"spaceId"`
				DegradeReason string `json:"degradeReason"`
				Strategy      string `json:"strategy"`
			} `json:"candidateSpaces"`
			Stages []struct {
				Name string `json:"name"`
			} `json:"stages"`
			PolicySnapshot map[string]string `json:"policy_version_snapshot"`
			DegradeCount   int               `json:"degradeCount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
	require.Equal(t, http.StatusOK, apiResp.Code)
	require.Len(t, apiResp.Data.CandidateSpaces, 2)
	require.NotEmpty(t, apiResp.Data.CandidateSpaces[0].SpaceID)
	require.Equal(t, "time-aware", apiResp.Data.CandidateSpaces[0].Strategy)
	require.GreaterOrEqual(t, apiResp.Data.DegradeCount, 1)
	require.NotEmpty(t, apiResp.Data.PolicySnapshot)
	require.Len(t, apiResp.Data.Stages, 5)

	// Memory snapshot should accept optional traceId and persist updates.
	memPayload, _ := json.Marshal(map[string]any{
		"sessionId": "qa-http-session",
		"traceId":   "trace-qa-http-1",
		"updates": []map[string]any{{
			"chunkId":    "chunk-1",
			"spaceId":    spaceA.UUID.String(),
			"citations":  []string{"doc#1"},
			"status":     "answered",
			"sourceType": "pdf",
			"confidence": 0.9,
		}},
	})
	memReq := httptest.NewRequest(http.MethodPost, "/api/openapi/knowledge-spaces/qa/memory-snapshot", bytes.NewReader(memPayload))
	memReq.Header.Set("Content-Type", "application/json")
	memResp := serveKnowledgeRequest(t, engine, memReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, memResp.Code)

	fetchPayload, _ := json.Marshal(map[string]any{
		"sessionId": "qa-http-session",
	})
	fetchReq := httptest.NewRequest(http.MethodPost, "/api/openapi/knowledge-spaces/qa/memory-snapshot", bytes.NewReader(fetchPayload))
	fetchReq.Header.Set("Content-Type", "application/json")
	fetchResp := serveKnowledgeRequest(t, engine, fetchReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, fetchResp.Code)

	var fetchData struct {
		Code int `json:"code"`
		Data struct {
			Citations []struct {
				ChunkID string `json:"chunkId"`
			} `json:"citations"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(fetchResp.Body.Bytes(), &fetchData))
	require.Equal(t, http.StatusOK, fetchData.Code)
	require.Len(t, fetchData.Data.Citations, 1)
	require.Equal(t, "chunk-1", fetchData.Data.Citations[0].ChunkID)
}

