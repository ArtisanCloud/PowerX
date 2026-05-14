package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDynamicRouter_PublicExposureBypassesAPIMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	authCalled := false
	dr := NewDynamicRouter("/_p", engine, func(c *gin.Context) {
		authCalled = true
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid Authorization header"})
	})
	dr.BindAuthorizer(nil, "powerx-auth", 0)
	dr.InstallPolicy("com.powerx.plugins.test", &Policy{
		PublicRoutes: []PublicRoute{
			{Method: http.MethodPost, Path: "/api/v1/integration/test/webhooks/shopify"},
		},
	})
	dr.MountAPIProxy("com.powerx.plugins.test", nil, "/api/v1", "/healthz")

	req := httptest.NewRequest(http.MethodPost, "/_p/com.powerx.plugins.test/api/v1/integration/test/webhooks/shopify", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if authCalled {
		t.Fatal("auth middleware should be bypassed for public exposure route")
	}
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("public route should not fail at host auth middleware, code=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-PowerX-Plugin-Public-Route") != "1" {
		t.Fatalf("missing public route response header")
	}
}

func TestDynamicRouter_NonPublicRouteUsesAPIMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	authCalled := false
	dr := NewDynamicRouter("/_p", engine, func(c *gin.Context) {
		authCalled = true
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid Authorization header"})
	})
	dr.BindAuthorizer(nil, "powerx-auth", 0)
	dr.InstallPolicy("com.powerx.plugins.test", &Policy{
		PublicRoutes: []PublicRoute{
			{Method: http.MethodPost, Path: "/api/v1/integration/test/webhooks/shopify"},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/_p/com.powerx.plugins.test/api/v1/integration/test/not-public", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if !authCalled {
		t.Fatal("auth middleware should run for non-public route")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d, want 401", rec.Code)
	}
}
