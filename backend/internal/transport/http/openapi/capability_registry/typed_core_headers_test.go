package capability_registry

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestTypedCoreBindingDoesNotReceiveProxyHeaders(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/v1/tenant/invocations", nil)
	ctx.Request.Header.Set("Authorization", "Bearer private-test-value")
	payload := map[string]interface{}{"endpoint": "core://knowledge/documents", "method": "INVOKE", "body": map[string]interface{}{"operation": "get_job"}}
	injectDefaultHeaders(payload, ctx)
	require.NotContains(t, payload, "headers")
	rest := map[string]interface{}{"endpoint": "/api/v1/tenant/knowledge/spaces"}
	injectDefaultHeaders(rest, ctx)
	require.Contains(t, rest, "headers")
}
