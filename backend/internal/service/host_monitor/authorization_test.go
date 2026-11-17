package hostmonitor_test

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/monitor"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestHostRouteRootAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name   string
		c      *reqctx.CoreXClaims
		root   bool
		query  string
		status int
	}{
		{"anonymous", nil, false, "", 403}, {"flag only", nil, true, "", 403},
		{"nonroot", &reqctx.CoreXClaims{UserID: 1, Scope: "access"}, false, "", 403},
		{"api key", &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access", Platforms: []string{"api_key"}}, true, "", 403},
		{"service", &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access", Platforms: []string{"service_actor"}}, true, "", 403},
		{"plugin", &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access", PluginID: "test"}, true, "", 403},
		{"root query", &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"}, true, "?path=/etc", 400},
		{"root", &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"}, true, "", 200},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			g := r.Group("/api/v1")
			monitor.RegisterAPIRoutes(nil, g, &shared.Deps{})
			req := httptest.NewRequest("GET", "/api/v1/admin/monitor/host"+tt.query, nil)
			ctx, cancel := context.WithCancel(req.Context())
			cancel()
			if tt.c != nil {
				ctx = reqctx.WithClaims(ctx, tt.c)
			}
			ctx = reqctx.WithIsRoot(ctx, tt.root)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.status || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}
