package integrationgatewaycontract

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
)

func serveIntegrationHTTPRequest(t testing.TB, handler http.Handler, req *http.Request, tenantUUID string) *httptest.ResponseRecorder {
	t.Helper()
	applyIntegrationHeaders(t, req, tenantUUID)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assertNoLegacyTenantPayload(t, resp.Body.Bytes())
	return resp
}

func applyIntegrationHeaders(t testing.TB, req *http.Request, tenantUUID string) {
	t.Helper()
	require.NotNil(t, req, "request must not be nil")
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer token")
	}
	if tenantUUID != "" {
		ctx := reqctx.WithTenantUUID(req.Context(), tenantUUID)
		*req = *req.WithContext(ctx)
	}
}

func assertNoLegacyTenantPayload(t testing.TB, body []byte) {
	t.Helper()
	if len(body) == 0 {
		return
	}
	payload := strings.ToLower(string(body))
	require.NotContains(t, payload, "tenant_id", "response leaked tenant_id field")
	require.NotContains(t, payload, "tenantid", "response leaked tenantId field")
}
