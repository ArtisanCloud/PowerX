package agentmodelhubcontract

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer token")
	}
	if tenantUUID == "" {
		tenantUUID = ammatestenv.AgentModelHubTenantUUID
	}
	ctx := reqctx.WithTenantUUID(req.Context(), tenantUUID)
	*req = *req.WithContext(ctx)
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
