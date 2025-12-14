package knowledge_space_contract

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func serveKnowledgeRequest(t testing.TB, handler http.Handler, req *http.Request, tenantUUID string) *httptest.ResponseRecorder {
	t.Helper()
	applyKnowledgeTenant(t, req, tenantUUID)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assertNoLegacyTenantPayload(t, resp.Body.Bytes())
	return resp
}

func applyKnowledgeTenant(t testing.TB, req *http.Request, tenantUUID string) {
	t.Helper()
	require.NotNil(t, req, "request is required")
	tenantUUID = strings.TrimSpace(tenantUUID)
	require.NotEmpty(t, tenantUUID, "tenant uuid required for knowledge space tests")
	require.Empty(t, strings.TrimSpace(req.Header.Get("X-Tenant-ID")), "legacy X-Tenant-ID header should not be sent")
	require.Empty(t, strings.TrimSpace(req.Header.Get("X-PowerX-Tenant")), "legacy X-PowerX-Tenant header should not be sent")

	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer token")
	}
	req.Header.Set("X-Tenant-UUID", tenantUUID)
}

func assertNoLegacyTenantPayload(t testing.TB, payload []byte) {
	t.Helper()
	if len(payload) == 0 {
		return
	}
	body := strings.ToLower(string(payload))
	require.NotContains(t, body, "tenant_id", "response leaked tenant_id field")
	require.NotContains(t, body, "tenantid", "response leaked tenantId field")
}
