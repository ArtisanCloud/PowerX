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
	env.Pipeline.WithInner(nil)

	policyID := env.SeedPolicyTemplate("http-feedback", "v1")
	space := env.CreateSpaceFixture("http-feedback-space", policyID)
	engine := env.Engine()

	traceID := "trace-http-123"
	body := map[string]any{
		"severity":     "high",
		"issueType":    "accuracy",
		"linkedChunks": []string{uuid.NewString()},
		"notes":        "答案不准确，请更新资料。",
		"reportedBy":   "qa@powerx.local",
		"toolTraceRef": traceID,
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/feedback", space.UUID), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusAccepted, resp.Code)

	var apiResp struct {
		Data struct {
			CaseID   string `json:"caseId"`
			Status   string `json:"status"`
			TraceID  string `json:"traceId"`
			TraceRef string `json:"toolTraceRef"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &apiResp))
	require.NotEmpty(t, apiResp.Data.CaseID)
	require.Equal(t, "in_progress", apiResp.Data.Status)
	require.Equal(t, traceID, apiResp.Data.TraceID)
	require.Equal(t, traceID, apiResp.Data.TraceRef)

	listReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/knowledge-spaces/%s/feedback", space.UUID), nil)
	listResp := serveKnowledgeRequest(t, engine, listReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, listResp.Code)

	var listData struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listData))
	require.NotEmpty(t, listData.Data)

	escalateBody, _ := json.Marshal(map[string]any{
		"requestedBy": "sre@powerx.local",
		"reason":      "需要人工复核并通知业务方",
	})
	escalateReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/feedback/%s/escalate", space.UUID, apiResp.Data.CaseID), bytes.NewReader(escalateBody))
	escalateReq.Header.Set("Content-Type", "application/json")
	escalateResp := serveKnowledgeRequest(t, engine, escalateReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, escalateResp.Code)

	closeBody, _ := json.Marshal(map[string]any{
		"requestedBy":     "sre@powerx.local",
		"resolutionNotes": "已完成热更新并确认效果",
	})
	closeReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/knowledge-spaces/%s/feedback/%s/close", space.UUID, apiResp.Data.CaseID), bytes.NewReader(closeBody))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp := serveKnowledgeRequest(t, engine, closeReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, closeResp.Code)

	filterReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/knowledge-spaces/%s/feedback?status=closed", space.UUID), nil)
	filterResp := serveKnowledgeRequest(t, engine, filterReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, filterResp.Code)

	exportReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/knowledge-spaces/%s/feedback/export?limit=10", space.UUID), nil)
	exportResp := serveKnowledgeRequest(t, engine, exportReq, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, exportResp.Code)

	var exportPayload struct {
		Data struct {
			Cases  []map[string]any `json:"cases"`
			Audits []map[string]any `json:"audits"`
			Meta   map[string]any   `json:"meta"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(exportResp.Body.Bytes(), &exportPayload))
	require.NotEmpty(t, exportPayload.Data.Cases)
	require.NotEmpty(t, exportPayload.Data.Audits)
	require.NotEmpty(t, exportPayload.Data.Meta)

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
