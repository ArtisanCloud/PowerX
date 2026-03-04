package agentmodelhubintegration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	ammatestenv "github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func serveAgentModelHubRequest(t testing.TB, handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	applyAgentModelHubHeaders(t, req)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	assertNoAgentModelHubTenantLeak(t, recorder.Body.Bytes())
	return recorder
}

func doAgentModelHubJSONRequest(t *testing.T, engine *gin.Engine, method, path string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err, "marshal payload")
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return serveAgentModelHubRequest(t, engine, req)
}

func applyAgentModelHubHeaders(t testing.TB, req *http.Request) {
	t.Helper()
	require.NotNil(t, req, "request cannot be nil")
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer token")
	}
	ctx := reqctx.WithTenantUUID(req.Context(), ammatestenv.AgentModelHubTenantUUID)
	*req = *req.WithContext(ctx)
}

func assertNoAgentModelHubTenantLeak(t testing.TB, payload []byte) {
	t.Helper()
	if len(payload) == 0 {
		return
	}
	body := strings.ToLower(string(payload))
	require.NotContains(t, body, "tenant_id", "response leaked tenant_id")
	require.NotContains(t, body, "tenantid", "response leaked tenantId")
}
