package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestPluginWSQueryBearerIsUnambiguousAndStrippedBeforeAuth(t *testing.T) {
	for _, tc := range []struct {
		name, query, header string
		upgrade             bool
		status              int
	}{
		{"upgrade", "authorization=Bearer+fixture", "", true, 204},
		{"duplicate", "authorization=Bearer+fixture&authorization=Bearer+other", "", true, 400},
		{"conflicting", "authorization=Bearer+fixture", "Bearer other", true, 400},
		{"invalid_scheme", "authorization=ApiKey+fixture", "", true, 400},
		{"ordinary_http", "authorization=Bearer+fixture", "", false, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			_ = NewDynamicRouter("/_p", engine, func(c *gin.Context) {
				if c.GetHeader("Authorization") != "Bearer fixture" {
					c.AbortWithStatus(401)
					return
				}
				require.Empty(t, c.Query("authorization"))
				require.NotContains(t, c.Request.RequestURI, "Bearer")
				c.AbortWithStatus(204)
			})
			r := httptest.NewRequest("GET", "/_p/com.example.fixture/api/v1/ws?"+tc.query, nil)
			if tc.upgrade {
				r.Header.Set("Upgrade", "websocket")
				r.Header.Set("Connection", "Upgrade")
			}
			if tc.header != "" {
				r.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
		})
	}
}
func TestPluginWSProxyRetainsProtectedIdentityAndUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth.SetJWTSecret([]byte("fixture-secret"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/ws", r.URL.Path)
		require.Empty(t, r.URL.Query().Get("authorization"))
		_, err := auth.ParseAndValidate(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), []byte("fixture-secret"), "powerx-auth", "plugin:com.example.fixture")
		require.NoError(t, err)
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		require.NoError(t, err)
		defer c.Close()
		require.NoError(t, c.WriteMessage(websocket.TextMessage, []byte(`{"type":"fixture_ready"}`)))
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	engine := gin.New()
	authenticated, guarded := false, false
	dr := NewDynamicRouter("/_p", engine, func(c *gin.Context) {
		require.Equal(t, "Bearer fixture", c.GetHeader("Authorization"))
		authenticated = true
		claims := reqctx.CoreXClaims{TenantUUID: "00000000-0000-4000-8000-000000000001", UserID: 101, MemberID: 202}
		ctx := reqctx.WithClaims(c.Request.Context(), &claims)
		ctx = reqctx.WithTenantUUID(ctx, claims.TenantUUID)
		ctx = reqctx.WithUserID(ctx, claims.UserID)
		ctx = reqctx.WithMemberID(ctx, claims.MemberID)
		c.Request = c.Request.WithContext(ctx)
		c.Set("auth_claims", claims)
		c.Next()
	})
	dr.BindAuthorizer(&registeredRouteAuthorizer{routes: map[string]Permission{"GET:/ws": {Resource: "runtime.contract", Action: "tenant_context"}}, perms: []string{"runtime.contract:tenant_context"}}, "powerx-auth", 0)
	dr.BindTenantPluginGuard(func(context.Context, string, string) error { guarded = true; return nil })
	dr.MountAPIProxy("com.example.fixture", target, "/api/v1", "/healthz")
	host := httptest.NewServer(engine)
	defer host.Close()
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(host.URL, "http")+"/_p/com.example.fixture/api/v1/ws?authorization=Bearer%20fixture", nil)
	require.NoError(t, err)
	defer conn.Close()
	require.Equal(t, 101, response.StatusCode)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"fixture_ready"}`, string(data))
	require.True(t, authenticated)
	require.True(t, guarded)
}
