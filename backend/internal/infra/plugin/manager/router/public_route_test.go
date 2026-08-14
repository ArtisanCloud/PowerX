package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
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
			{Method: http.MethodPost, Path: "/integration/test/webhooks/callback"},
		},
	})
	dr.MountAPIProxy("com.powerx.plugins.test", nil, "/api/v1", "/healthz")

	req := httptest.NewRequest(http.MethodPost, "/_p/com.powerx.plugins.test/api/v1/integration/test/webhooks/callback", nil)
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
			{Method: http.MethodPost, Path: "/integration/test/webhooks/callback"},
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

func TestDynamicRouter_RuntimeWSBusRouteRequiresRegisteredPermissionSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth.SetJWTSecret([]byte("test-secret"))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/runtime/ws-bus/grant" {
			t.Fatalf("upstream path = %q", r.URL.Path)
		}
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			t.Fatalf("missing plugin token authorization header: %q", authz)
		}
		claims, err := auth.ParseAndValidate(strings.TrimPrefix(authz, "Bearer "), []byte("test-secret"), "powerx-auth", "plugin:com.powerx.plugins.test")
		if err != nil {
			t.Fatalf("ParseAndValidate error: %v", err)
		}
		if len(claims.PermissionCodes) != 1 || claims.PermissionCodes[0] != "runtime.ops:manage" {
			t.Fatalf("permission codes = %#v, want runtime.ops:manage", claims.PermissionCodes)
		}
		if claims.PolicyVersion == "" {
			t.Fatalf("expected policy_version in delegated snapshot")
		}
		if claims.PermsHash == "" {
			t.Fatalf("expected perms_hash in delegated snapshot")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}

	engine := gin.New()
	tenantUUID := "6b5d0240-9920-46da-b707-88200e0f51ea"
	dr := NewDynamicRouter("/_p", engine, func(c *gin.Context) {
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
	dr.BindAuthorizer(&registeredRouteAuthorizer{
		routes: map[string]Permission{
			"POST:/admin/runtime/ws-bus/grant": {
				Resource: "runtime.ops",
				Action:   "manage",
			},
		},
		perms: []string{"runtime.ops:manage"},
	}, "powerx-auth", 0)
	dr.BindTenantPluginGuard(func(context.Context, string, string) error { return nil })
	dr.MountAPIProxy("com.powerx.plugins.test", upstreamURL, "/api/v1", "/healthz")

	host := httptest.NewServer(engine)
	t.Cleanup(host.Close)

	resp, err := http.Post(host.URL+"/_p/com.powerx.plugins.test/api/v1/admin/runtime/ws-bus/grant", "application/json", nil)
	if err != nil {
		t.Fatalf("post runtime route: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("code=%d, want 204", resp.StatusCode)
	}
}

func TestDynamicRouter_GatewayDenyResponseIncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth.SetJWTSecret([]byte("test-secret"))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("upstream should not be called")
	}))
	t.Cleanup(upstream.Close)
	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}

	engine := gin.New()
	dr := NewDynamicRouter("/_p", engine, func(c *gin.Context) {
		claims := reqctx.CoreXClaims{
			TenantUUID: "tenant-uuid",
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
	dr.BindAuthorizer(&registeredRouteAuthorizer{
		routes: map[string]Permission{
			"POST:/admin/example/records": {
				Module:   "example",
				Resource: "record",
				Action:   "create",
			},
		},
		perms: []string{"example.record:read"},
	}, "powerx-auth", 0)
	dr.BindTenantPluginGuard(func(context.Context, string, string) error { return nil })
	dr.MountAPIProxy("com.powerx.plugins.test", upstreamURL, "/api/v1", "/healthz")

	req := httptest.NewRequest(http.MethodPost, "/_p/com.powerx.plugins.test/api/v1/admin/example/records", nil)
	req.Header.Set("X-Request-ID", "req-test-123")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d, want 403 body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json body error: %v body=%s", err, rec.Body.String())
	}
	if body["request_id"] != "req-test-123" {
		t.Fatalf("request_id=%v, want req-test-123 body=%v", body["request_id"], body)
	}
	if body["trace_id"] == "" {
		t.Fatalf("expected trace_id body=%v", body)
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
