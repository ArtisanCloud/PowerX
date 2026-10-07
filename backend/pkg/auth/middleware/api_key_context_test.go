package middleware

import (
	apikeycache "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeycache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestCachedAPIKeyKeepsAuthenticatedKeyIdentityInRequestContext(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/tenant/capabilities:grant-status", nil)
	c.Request.Header.Set("X-API-Key-Hash", "untrusted-test-value")
	require.Empty(t, reqctx.AuthenticatedAPIKeyHash(c.Request.Context()))
	snapshot := &apikeycache.AuthSnapshot{TenantUUID: "11111111-1111-1111-1111-111111111111", ProfileID: 1, KeyID: 1}
	require.True(t, applyCachedAPIKeyContext(c, jwtMiddlewareConfig{}, snapshot, "verified-test-hash"))
	require.Equal(t, "verified-test-hash", reqctx.AuthenticatedAPIKeyHash(c.Request.Context()))
	require.Equal(t, c.GetString("auth_api_key_hash"), reqctx.AuthenticatedAPIKeyHash(c.Request.Context()))
}
