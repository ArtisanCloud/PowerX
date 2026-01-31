package agentmodelhubcontract

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ammatestenv "github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/stretchr/testify/require"
)

func serveAgentModelHubRequest(t testing.TB, handler http.Handler, req *http.Request, tenantUUID string) *httptest.ResponseRecorder {
	t.Helper()
	applyAgentModelHubHeaders(t, req, tenantUUID)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assertNoAMHTenantLeak(t, resp.Body.Bytes())
	return resp
}

func applyAgentModelHubHeaders(t testing.TB, req *http.Request, tenantUUID string) {
	t.Helper()
	require.NotNil(t, req, "request cannot be nil")
	require.Empty(t, strings.TrimSpace(req.Header.Get("X-Tenant-ID")), "legacy X-Tenant-ID header forbidden")
	require.Empty(t, strings.TrimSpace(req.Header.Get("X-PowerX-Tenant")), "legacy X-PowerX-Tenant header forbidden")
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer token")
	}
	if tenantUUID == "" {
		tenantUUID = ammatestenv.AgentModelHubTenantUUID
	}
	req.Header.Set("X-PowerX-Tenant", tenantUUID)
}

func assertNoAMHTenantLeak(t testing.TB, payload []byte) {
	t.Helper()
	if len(payload) == 0 {
		return
	}
	body := strings.ToLower(string(payload))
	require.NotContains(t, body, "tenant_id", "response leaked tenant_id")
	require.NotContains(t, body, "tenantid", "response leaked tenantId")
}
