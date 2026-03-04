package plugin_release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func requirePluginAdminAuth(tenantUUID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		headerUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
		if headerUUID == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ctx := reqctx.WithClaims(c.Request.Context(), &reqctx.CoreXClaims{IsRoot: true, Roles: []string{"system_admin"}})
		ctx = reqctx.WithTenantUUID(ctx, headerUUID)
		c.Request = c.Request.WithContext(ctx)
		reqctx.CopyCtxToGin(c)
		c.Next()
	}
}

func servePluginAdminRequest(t testing.TB, handler http.Handler, req *http.Request, tenantUUID string) *httptest.ResponseRecorder {
	t.Helper()
	applyPluginAdminHeaders(t, req, tenantUUID)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assertNoPluginTenantLeak(t, resp.Body.Bytes())
	return resp
}

func servePluginTenantRequest(t testing.TB, handler http.Handler, req *http.Request, tenantUUID string) *httptest.ResponseRecorder {
	t.Helper()
	applyPluginTenantHeaders(t, req, tenantUUID)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assertNoPluginTenantLeak(t, resp.Body.Bytes())
	return resp
}

func applyPluginAdminHeaders(t testing.TB, req *http.Request, tenantUUID string) {
	t.Helper()
	require.NotNil(t, req, "request cannot be nil")
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer admin")
	}
	ctx := reqctx.WithTenantUUID(req.Context(), tenantUUID)
	*req = *req.WithContext(ctx)
}

func applyPluginTenantHeaders(t testing.TB, req *http.Request, tenantUUID string) {
	t.Helper()
	require.NotNil(t, req, "request cannot be nil")
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer tenant")
	}
	ctx := reqctx.WithTenantUUID(req.Context(), tenantUUID)
	*req = *req.WithContext(ctx)
}

func assertNoPluginTenantLeak(t testing.TB, payload []byte) {
	t.Helper()
	if len(payload) == 0 {
		return
	}
	body := strings.ToLower(string(payload))
	require.NotContains(t, body, "tenant_id", "response leaked tenant_id")
	require.NotContains(t, body, "tenantid", "response leaked tenantId")
}

func pluginReleaseGRPCContext(t testing.TB, parent context.Context, tenantUUID string) context.Context {
	t.Helper()
	if parent == nil {
		parent = context.Background()
	}
	if tenantUUID == "" {
		tenantUUID = "plugin-release-grpc"
	}
	md := metadata.New(map[string]string{
		"tenant-uuid": tenantUUID,
		"authorization": "Bearer token",
	})
	return metadata.NewOutgoingContext(parent, md)
}

func assertNoPluginReleaseTenantLeakProto(t testing.TB, msg proto.Message) {
	t.Helper()
	if msg == nil {
		return
	}
	data, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	require.NoError(t, err)
	assertNoPluginTenantLeak(t, data)
}
