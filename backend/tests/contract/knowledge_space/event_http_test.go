package knowledge_space_contract

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
)

func TestEventHotfixHTTP(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	secret := "test-secret"
	t.Setenv("PX_KNOWLEDGE_EVENT_SIGNATURE_SECRET", secret)
	t.Setenv("PX_KNOWLEDGE_EVENT_SIGNATURE_HEADER", "X-PowerX-Signature")
	t.Setenv("PX_KNOWLEDGE_EVENT_TIMESTAMP_HEADER", "X-PowerX-Timestamp")
	t.Setenv("PX_KNOWLEDGE_EVENT_ALLOWED_SKEW_SECONDS", "300")

	engine := env.Engine()
	payload := map[string]any{
		"eventId":   "evt-http-1",
		"eventType": "policy-update",
		"payload":   map[string]any{"tenant": env.TenantUUID().String()},
	}
	body, _ := json.Marshal(payload)
	req := signedRequest(t, http.MethodPost, "/api/knowledge/events/apply", body, secret)
	req.Header.Set("Content-Type", "application/json")
	resp := serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, resp.Code)

	req = signedRequest(t, http.MethodPost, "/api/knowledge/events/apply", body, secret)
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
	req = signedRequest(t, http.MethodPost, "/api/knowledge/events/retry", body, secret)
	req.Header.Set("Content-Type", "application/json")
	resp = serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, resp.Code)

	oldPayload := map[string]any{
		"eventId":    "evt-http-old",
		"eventType":  "policy-update",
		"payload":    map[string]any{"tenant": env.TenantUUID().String()},
		"receivedAt": time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
	}
	body, _ = json.Marshal(oldPayload)
	req = signedRequest(t, http.MethodPost, "/api/knowledge/events/apply", body, secret)
	req.Header.Set("Content-Type", "application/json")
	resp = serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusBadRequest, resp.Code)

	hotUpdatePayload := map[string]any{
		"eventId": "evt-http-hotupdate",
		"spaceId": env.CreateSpaceFixture("event-http-space", env.SeedPolicyTemplate("event-http", "v1")).UUID.String(),
		"payload": map[string]any{"reason": "manual"},
	}
	body, _ = json.Marshal(hotUpdatePayload)
	req = signedRequest(t, http.MethodPost, "/api/knowledge/index/hot-update", body, secret)
	req.Header.Set("Content-Type", "application/json")
	resp = serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, resp.Code)

	refreshBody := []byte(`{"targetEventType":"policy-update"}`)
	req = signedRequest(t, http.MethodPost, "/api/agent/weights/refresh", refreshBody, secret)
	req.Header.Set("Content-Type", "application/json")
	resp = serveKnowledgeRequest(t, engine, req, env.TenantUUID().String())
	require.Equal(t, http.StatusOK, resp.Code)

	unsigned := httptest.NewRequest(http.MethodPost, "/api/knowledge/events/apply", bytes.NewReader([]byte(`{}`)))
	unsigned.Header.Set("Content-Type", "application/json")
	resp = serveKnowledgeRequest(t, engine, unsigned, env.TenantUUID().String())
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

func signedRequest(t *testing.T, method, url string, body []byte, secret string) *http.Request {
	t.Helper()
	ts := time.Now().UTC().Unix()
	tsRaw := strconv.FormatInt(ts, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(tsRaw))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	req := httptest.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("X-PowerX-Timestamp", tsRaw)
	req.Header.Set("X-PowerX-Signature", sig)
	return req
}
