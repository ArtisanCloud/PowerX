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

func TestEventHotfixHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	engine := env.Engine()
	payload := map[string]any{
		"eventId":   "evt-http-1",
		"eventType": "policy-update",
		"payload":   map[string]any{"tenant": env.TenantUUID().String()},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/knowledge/events/apply", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, resp.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/knowledge/events/apply", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp = serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusConflict, resp.Code)

	retryPayload := map[string]any{
		"eventId":    "evt-http-retry",
		"eventType":  "policy-update",
		"payload":    map[string]any{"tenant": env.TenantUUID().String()},
		"retryCount": 1,
	}
	body, _ = json.Marshal(retryPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/knowledge/events/retry", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp = serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, resp.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/knowledge/index/hot-update", bytes.NewReader([]byte(`{}`)))
	resp = serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, resp.Code)
}
