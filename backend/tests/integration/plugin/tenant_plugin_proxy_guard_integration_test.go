package pluginintegration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	pluginrouter "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
}

func (r closeNotifyRecorder) CloseNotify() <-chan bool {
	ch := make(chan bool, 1)
	return ch
}

func TestTenantPluginProxyGuardDeniesDisabledTenantInstance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamURL, err := url.Parse("http://127.0.0.1:1")
	require.NoError(t, err)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		claims := reqctx.CoreXClaims{
			TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
			UserID:     101,
			MemberID:   202,
		}
		ctx := reqctx.WithClaims(c.Request.Context(), &claims)
		ctx = reqctx.WithTenantUUID(ctx, claims.TenantUUID)
		ctx = reqctx.WithUserID(ctx, claims.UserID)
		ctx = reqctx.WithMemberID(ctx, claims.MemberID)
		c.Request = c.Request.WithContext(ctx)
		c.Set("auth_claims", claims)
		c.Next()
	})

	dr := pluginrouter.NewDynamicRouter("/_p", engine)
	dr.MountAdminProxy("com.powerx.plugins.disabled", upstreamURL)
	dr.MountAPIProxy("com.powerx.plugins.disabled", upstreamURL, "/api/v1", "/healthz")

	adminRec := closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	adminReq := httptest.NewRequest(http.MethodGet, "/_p/com.powerx.plugins.disabled/admin/", nil)
	adminReq.Header.Set("Accept", "text/html")
	engine.ServeHTTP(adminRec, adminReq)

	require.Equal(t, http.StatusForbidden, adminRec.Code, "disabled tenant plugin admin entry must be denied before upstream")

	apiRec := closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	apiReq := httptest.NewRequest(http.MethodGet, "/_p/com.powerx.plugins.disabled/api/v1/admin/ping", nil)
	engine.ServeHTTP(apiRec, apiReq)

	require.Equal(t, http.StatusForbidden, apiRec.Code, "disabled tenant plugin api entry must be denied before upstream")
}

func TestTenantPluginProxyGuardRequiresTenantContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamURL, err := url.Parse("http://127.0.0.1:1")
	require.NoError(t, err)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.Background())
		c.Next()
	})

	dr := pluginrouter.NewDynamicRouter("/_p", engine)
	dr.MountAdminProxy("com.powerx.plugins.test", upstreamURL)
	dr.MountAPIProxy("com.powerx.plugins.test", upstreamURL, "/api/v1", "/healthz")

	rec := closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodGet, "/_p/com.powerx.plugins.test/admin/", nil)
	req.Header.Set("Accept", "text/html")
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code, "plugin admin entry must fail fast without tenant context")
}
