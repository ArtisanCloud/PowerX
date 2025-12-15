package knowledge_space_contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
)

func TestQABridgeRetrievalPlanHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	tpl := env.SeedPolicyTemplate("qa-plan", "v1")
	spaceA := env.CreateSpaceFixture("qa-alpha", tpl)
	spaceB := env.CreateSpaceFixture("qa-beta", tpl)
	require.NoError(t, env.ActivateSpace(spaceA.UUID))
	require.NoError(t, env.ActivateSpace(spaceB.UUID))

	engine := env.Engine()

	body := map[string]any{
		"intent":          "供应商是否超限",
		"domainTags":      []string{"finance", "policy"},
		"latencyBudgetMs": 1500,
		"sessionId":       "session-alpha",
	}

	req := newJSONRequest(http.MethodPost, "/api/openapi/knowledge-spaces/qa/retrieval-plan", body, env.TenantUUID().String())
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var apiResp struct {
		Code int               `json:"code"`
		Data qaPlanHTTPPayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
	require.Equal(t, http.StatusOK, apiResp.Code)
	require.Len(t, apiResp.Data.CandidateSpaces, 2)
	require.Equal(t, spaceA.UUID.String(), apiResp.Data.CandidateSpaces[0].SpaceID)
	require.Empty(t, apiResp.Data.CandidateSpaces[0].DegradeReason)
	require.Equal(t, "hybrid", apiResp.Data.CandidateSpaces[0].Strategy)
	require.NotEmpty(t, apiResp.Data.Telemetry.TraceID)

	// Trigger degrade path by retiring spaceB.
	require.NoError(t, env.SetSpaceStatus(spaceB.UUID, "retired"))
	req = newJSONRequest(http.MethodPost, "/api/openapi/knowledge-spaces/qa/retrieval-plan", body, env.TenantUUID().String())
	resp = httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	apiResp = struct {
		Code int               `json:"code"`
		Data qaPlanHTTPPayload `json:"data"`
	}{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
	require.Equal(t, http.StatusOK, apiResp.Code)
	require.Equal(t, 2, len(apiResp.Data.CandidateSpaces))
	require.Contains(t, apiResp.Data.CandidateSpaces[1].DegradeReason, "retired")
	require.Positive(t, apiResp.Data.DegradeCount)
}

func TestQABridgeMemorySnapshotHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	tpl := env.SeedPolicyTemplate("qa-memory", "v1")
	space := env.CreateSpaceFixture("qa-memory", tpl)
	require.NoError(t, env.ActivateSpace(space.UUID))

	engine := env.Engine()

	sessionID := "session-" + uuid.NewString()
	updates := []map[string]any{
		{
			"chunkId":     "chunk-001",
			"spaceId":     space.UUID.String(),
			"sourceType":  "pdf",
			"status":      "answered",
			"citations":   []string{"docA#1"},
			"confidence":  0.93,
			"deltaReason": "initial_answer",
		},
		{
			"chunkId":     "chunk-002",
			"spaceId":     space.UUID.String(),
			"sourceType":  "table",
			"status":      "stale",
			"citations":   []string{"tableB#3"},
			"confidence":  0.61,
			"deltaReason": "stale",
		},
	}
	payload := map[string]any{
		"sessionId": sessionID,
		"updates":   updates,
	}

	req := newJSONRequest(http.MethodPost, "/api/openapi/knowledge-spaces/qa/memory-snapshot", payload, env.TenantUUID().String())
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var snapshotResp struct {
		Code int                  `json:"code"`
		Data qaMemoryHTTPResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &snapshotResp))
	require.Equal(t, http.StatusOK, snapshotResp.Code)
	require.Equal(t, sessionID, snapshotResp.Data.SessionID)
	require.Len(t, snapshotResp.Data.Citations, 2)

	// Fetch again without updates to validate cached snapshot.
	payload = map[string]any{
		"sessionId": sessionID,
	}
	req = newJSONRequest(http.MethodPost, "/api/openapi/knowledge-spaces/qa/memory-snapshot", payload, env.TenantUUID().String())
	resp = httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &snapshotResp))
	require.Equal(t, 2, len(snapshotResp.Data.Citations))
	require.Equal(t, "chunk-001", snapshotResp.Data.Citations[0].ChunkID)
	require.Equal(t, "answered", snapshotResp.Data.Citations[0].Status)
}

type qaPlanHTTPPayload struct {
	TenantUUID      string                 `json:"tenant_uuid"`
	Intent          string                 `json:"intent"`
	DomainTags      []string               `json:"domainTags"`
	CandidateSpaces []qaCandidateSpace     `json:"candidateSpaces"`
	Tooling         []qaToolMetadataView   `json:"tooling"`
	Telemetry       qaPlanTelemetry        `json:"telemetry"`
	DegradeCount    int                    `json:"degradeCount"`
	SessionID       string                 `json:"sessionId"`
	LatencyBudgetMs int                    `json:"latencyBudgetMs"`
	Metadata        map[string]interface{} `json:"metadata"`
}

type qaCandidateSpace struct {
	SpaceID          string  `json:"spaceId"`
	SpaceName        string  `json:"spaceName"`
	Strategy         string  `json:"strategy"`
	CitationCoverage float64 `json:"citationCoverage"`
	DegradeReason    string  `json:"degradeReason"`
}

type qaPlanTelemetry struct {
	TraceID    string `json:"traceId"`
	RecordedAt string `json:"recordedAt"`
}

type qaToolMetadataView struct {
	ToolID   string `json:"toolId"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Endpoint string `json:"endpoint"`
}

type qaMemoryHTTPResponse struct {
	TenantUUID string              `json:"tenant_uuid"`
	SessionID  string              `json:"sessionId"`
	Citations  []qaCitationSummary `json:"citations"`
}

type qaCitationSummary struct {
	ChunkID     string   `json:"chunkId"`
	SpaceID     string   `json:"spaceId"`
	Status      string   `json:"status"`
	Citations   []string `json:"citations"`
	SourceType  string   `json:"sourceType"`
	Confidence  float64  `json:"confidence"`
	DeltaReason string   `json:"deltaReason"`
}
