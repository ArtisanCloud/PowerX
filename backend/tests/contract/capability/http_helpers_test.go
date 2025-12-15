package capabilitycontract

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func serveCapabilityAdminRequest(t testing.TB, handler http.Handler, req *http.Request, tenantUUID string) *httptest.ResponseRecorder {
	t.Helper()
	applyCapabilityAdminHeaders(t, req, tenantUUID)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assertNoCapabilityTenantLeak(t, resp.Body.Bytes())
	return resp
}

func applyCapabilityAdminHeaders(t testing.TB, req *http.Request, tenantUUID string) {
	t.Helper()
	require.NotNil(t, req, "request cannot be nil")
	requireCapabilityNoLegacyHeaders(t, req)
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer admin")
	}
	req.Header.Set("X-Tenant-UUID", tenantUUID)
}

func requireCapabilityNoLegacyHeaders(t testing.TB, req *http.Request) {
	t.Helper()
	require.Empty(t, strings.TrimSpace(req.Header.Get("X-Tenant-ID")), "legacy X-Tenant-ID header forbidden")
	require.Empty(t, strings.TrimSpace(req.Header.Get("X-PowerX-Tenant")), "legacy X-PowerX-Tenant header forbidden")
	require.Empty(t, strings.TrimSpace(req.Header.Get("Tenant-ID")), "legacy Tenant-ID header forbidden")
}

func assertNoCapabilityTenantLeak(t testing.TB, payload []byte) {
	t.Helper()
	if len(payload) == 0 {
		return
	}
	body := strings.ToLower(string(payload))
	require.NotContains(t, body, "tenant_id", "response leaked tenant_id")
	require.NotContains(t, body, "tenantid", "response leaked tenantId")
}

func requireCapabilityAuth(expectedTenantUUID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		tenantUUID := strings.TrimSpace(c.GetHeader("X-Tenant-UUID"))
		if tenantUUID == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if expectedTenantUUID != "" && !strings.EqualFold(expectedTenantUUID, tenantUUID) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
		c.Request = c.Request.WithContext(ctx)
		reqctx.CopyCtxToGin(c)
		c.Next()
	}
}
