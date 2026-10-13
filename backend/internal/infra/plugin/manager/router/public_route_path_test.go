package router

import "testing"

func TestNormalizePublicRoutePath(t *testing.T) {
	for _, tc := range []struct{ name, path, base, want string }{
		{"declared full path", "/api/v1/integration/callback", "/api/v1", "/integration/callback"},
		{"gateway alias", "/v1/integration/callback", "/api/v1", "/integration/callback"},
		{"relative resource", "/integration/callback", "/api/v1", "/integration/callback"},
		{"v2 gateway alias", "/v2/integration/callback", "/api/v2", "/integration/callback"},
		{"custom HTTP base", "/custom/v3/integration/callback", "/custom/v3", "/integration/callback"},
		{"root HTTP base", "/integration/callback", "/", "/integration/callback"},
		{"base formatting", "api/v2/integration/callback", "api/v2/", "/integration/callback"},
		{"exact base", "/api/v2", "/api/v2", "/"},
		{"similar prefix stays distinct", "/api/v10/integration/callback", "/api/v1", "/api/v10/integration/callback"},
		{"empty path", "", "/api/v1", ""},
		{"empty segment", "/v1/sessions//events", "/api/v1", ""},
		{"dot segment", "/v1/sessions/./events", "/api/v1", ""},
		{"parent segment", "/v1/sessions/a/../b/events", "/api/v1", ""},
		{"encoded segment separator", "/v1/sessions/a%2Fb/events", "/api/v1", ""},
		{"encoded dot", "/v1/sessions/%2E/events", "/api/v1", ""},
		{"backslash", "/v1/sessions/a\\b/events", "/api/v1", ""},
		{"query is not route", "/v1/sessions/a/events?signature=x", "/api/v1", ""},
		{"trailing empty segment", "/v1/integration/callback/", "/api/v1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizePublicRoutePath(tc.path, tc.base); got != tc.want {
				t.Fatalf("NormalizePublicRoutePath(%q, %q)=%q want=%q", tc.path, tc.base, got, tc.want)
			}
		})
	}
}

func TestIsPublicRouteMatchesOnlyDeclaredSegmentsAndMethods(t *testing.T) {
	gate := newAuthzGate(nil, "test", 0)
	gate.InstallPolicy("installed", &Policy{PublicRoutes: []PublicRoute{
		{Method: "GET", Path: "/sessions/{sessionID}/events"},
		{Method: "POST", Path: "/sessions/:sessionID/messages"},
		{Method: "POST", Path: "/static/[literal]"},
		{Method: "GET", Path: "/"},
	}})
	for _, tc := range []struct {
		name, plugin, method, path string
		want                       bool
	}{
		{"brace parameter", "installed", "GET", "/sessions/abc/events", true},
		{"colon parameter", "installed", "POST", "/sessions/abc/messages", true},
		{"wrong method", "installed", "POST", "/sessions/abc/events", false},
		{"HEAD is separate", "installed", "HEAD", "/sessions/abc/events", false},
		{"wrong plugin", "other", "GET", "/sessions/abc/events", false},
		{"empty parameter", "installed", "GET", "/sessions//events", false},
		{"extra level", "installed", "GET", "/sessions/abc/extra/events", false},
		{"missing level", "installed", "GET", "/sessions/events", false},
		{"static case differs", "installed", "GET", "/Sessions/abc/events", false},
		{"dot path cannot collapse", "installed", "GET", "/sessions/extra/../abc/events", false},
		{"encoded separator", "installed", "GET", "/sessions/abc%2Fextra/events", false},
		{"literal brackets", "installed", "POST", "/static/[literal]", true},
		{"no glob expansion", "installed", "POST", "/static/l", false},
		{"declared root", "installed", "GET", "/", true},
		{"empty request cannot become root", "installed", "GET", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := gate.IsPublicRoute(tc.plugin, tc.method, tc.path); got != tc.want {
				t.Fatalf("IsPublicRoute(%q, %q, %q)=%v want=%v", tc.plugin, tc.method, tc.path, got, tc.want)
			}
		})
	}
}
