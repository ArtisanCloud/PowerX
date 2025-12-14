package knowledge_space_contract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestFeedbackHTTPHandlers(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	policyID := env.SeedPolicyTemplate("http-feedback", "v1")
	space := env.CreateSpaceFixture("http-feedback-space", policyID)
	engine := env.Engine()

	body := map[string]any{
		"severity":     "high",
		"issueType":    "accuracy",
		"linkedChunks": []string{uuid.NewString()},
		"notes":        "答案不准确，请更新资料。",
		"reportedBy":   "qa@powerx.local",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/feedback", space.UUID), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusAccepted, resp.Code)

	var apiResp struct {
		Data struct {
			CaseID string `json:"caseId"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
	require.NotEmpty(t, apiResp.Data.CaseID)
	require.Equal(t, "in_progress", apiResp.Data.Status)

	listReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/knowledge-spaces/%s/feedback", space.UUID), nil)
	listResp := serveKnowledgeRequest(t, engine, listReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, listResp.Code)

	var listData struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listData))
	require.NotEmpty(t, listData.Data)

	// Retire space and ensure feedback is rejected.
	_, err := env.Deps.KnowledgeSpace.Service.RetireSpace(context.Background(), ksvc.RetireSpaceInput{
		SpaceID: space.UUID,
	})
	require.NoError(t, err)
	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/feedback", space.UUID), bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	resp2 := serveKnowledgeRequest(t, engine, req2, env.TenantUUID().String())
	require.Equal(t, http.StatusGone, resp2.Code)
}
