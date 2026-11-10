package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	identity "github.com/ArtisanCloud/PowerX/internal/service/plugin_identity"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIdentityDiagnosticErrorPreservesPublicHTTPEnvelope(t *testing.T) {
	r := &DynamicRouter{}
	r.BindRequestIdentityIssuer(identity.NewService(nil, nil, identity.Options{}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/_p/com.powerx.plugins.fixture/api/v1/orders", nil)
	ticket, issued := r.issueRequestIdentity(c, identity.IssueInput{
		PluginID: "com.powerx.plugins.fixture", Method: http.MethodGet, Path: "/admin/orders",
		RequestID: "diagnostic-request", TraceID: "diagnostic-trace", Transport: "http",
		Claims: reqctx.CoreXClaims{},
	})
	require.Empty(t, ticket)
	require.False(t, issued)
	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	for _, field := range []string{"message", "error", "error_code", "reason_code"} {
		require.Equal(t, "IAM_CONTEXT_UNAVAILABLE", body[field])
	}
	require.NotContains(t, body, "details")
	require.NotContains(t, w.Body.String(), "service_not_ready")
	require.NotContains(t, w.Body.String(), "session_ref")
}
