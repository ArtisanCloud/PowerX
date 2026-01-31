package eventfabric

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const eventFabricTestTenantUUID = "tenant-corex"

func attachTenantContext(group *gin.RouterGroup, tenantUUID string) {
	key := string(reqctx.KeyTenantUUID)
	group.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
		c.Request = c.Request.WithContext(ctx)
		c.Set(key, tenantUUID)
		c.Next()
	})
}

func httpRequest(t *testing.T, handler http.Handler, method, path string, body interface{}) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err, "marshal request body")
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	applyEventFabricHeaders(t, req, eventFabricTestTenantUUID)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body")
	_ = resp.Body.Close()
	assertNoEventFabricTenantLeak(t, data)
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return resp
}

func decodeJSON(t *testing.T, body io.ReadCloser, out interface{}) {
	t.Helper()
	defer body.Close()
	decoder := json.NewDecoder(body)
	require.NoError(t, decoder.Decode(out), "decode response body")
}

func applyEventFabricHeaders(t testing.TB, req *http.Request, tenantUUID string) {
	t.Helper()
	require.NotNil(t, req, "request must not be nil")
	require.Empty(t, strings.TrimSpace(req.Header.Get("X-Tenant-ID")), "legacy X-Tenant-ID header forbidden")
	require.Empty(t, strings.TrimSpace(req.Header.Get("X-PowerX-Tenant")), "legacy X-PowerX-Tenant header forbidden")
	require.Empty(t, strings.TrimSpace(req.Header.Get("Tenant-ID")), "legacy Tenant-ID header forbidden")
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer admin")
	}
	req.Header.Set("X-PowerX-Tenant", strings.TrimSpace(tenantUUID))
}

func assertNoEventFabricTenantLeak(t testing.TB, payload []byte) {
	t.Helper()
	if len(payload) == 0 {
		return
	}
	body := strings.ToLower(string(payload))
	require.NotContains(t, body, "tenant_id", "response leaked tenant_id")
	require.NotContains(t, body, "tenantid", "response leaked tenantId")
}
