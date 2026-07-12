package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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

func TestDynamicRouter_AdminRouteUsesAPIMiddlewareBeforeTenantGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	authCalled := false
	guardCalled := false
	tenantUUID := "6b5d0240-9920-46da-b707-88200e0f51ea"

	dr := NewDynamicRouter("/_p", engine, func(c *gin.Context) {
		authCalled = true
		claims := reqctx.CoreXClaims{
			TenantUUID: tenantUUID,
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
	dr.BindTenantPluginGuard(func(ctx context.Context, gotTenantUUID, pluginID string) error {
		guardCalled = true
		if gotTenantUUID != tenantUUID {
			t.Fatalf("tenant uuid = %q, want %q", gotTenantUUID, tenantUUID)
		}
		if pluginID != "com.powerx.plugins.test" {
			t.Fatalf("plugin id = %q", pluginID)
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/_p/com.powerx.plugins.test/admin/", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if !authCalled {
		t.Fatal("admin route should run auth middleware before tenant guard")
	}
	if !guardCalled {
		t.Fatal("admin route should run tenant guard with auth context")
	}
	if rec.Code == http.StatusForbidden {
		t.Fatalf("admin route should not fail tenant guard after auth context injection, body=%s", rec.Body.String())
	}
}
