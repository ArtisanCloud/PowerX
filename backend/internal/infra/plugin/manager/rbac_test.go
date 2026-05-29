package manager

import (
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func TestToPolicyHTTPBase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "keep api v1", in: "/api/v1", want: "/api/v1"},
		{name: "keep v1", in: "/v1", want: "/v1"},
		{name: "prepend leading slash", in: "api/v2", want: "/api/v2"},
		{name: "empty", in: "", want: ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := toPolicyHTTPBase(tc.in)
			if got != tc.want {
				t.Fatalf("toPolicyHTTPBase(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPolicyFromPlugin_AddWSBusRoutes(t *testing.T) {
	t.Parallel()

	p := plugin_mgr.Plugin{
		Routes: &plugin_mgr.RouteSpec{
			BasePath: "/api/v1",
		},
		RBAC: plugin_mgr.RBACSpec{
			Resources: []plugin_mgr.RBACResource{
				{
					Resource: "base:runtime",
					Actions:  []string{"create", "read", "update"},
				},
			},
		},
	}

	pol := PolicyFromPlugin(p)
	if pol == nil {
		t.Fatalf("PolicyFromPlugin returned nil")
	}

	routes := []string{
		"POST:/api/v1/admin/runtime/ws-bus/test-flow",
		"POST:/api/v1/admin/runtime/ws-bus/grant",
		"POST:/api/v1/admin/runtime/ws-bus/publish",
	}
	for _, k := range routes {
		perm, ok := pol.Routes[k]
		if !ok {
			t.Fatalf("expected route %s in policy routes", k)
		}
		if perm.Resource == "" || perm.Action == "" {
			t.Fatalf("route %s permission invalid: %+v", k, perm)
		}
	}
}

func TestPolicyFromPlugin_AddPublicExposureRoutes(t *testing.T) {
	t.Parallel()

	p := plugin_mgr.Plugin{
		Endpoints: plugin_mgr.EndpointSpec{HTTPBasePath: "/api/v1"},
		Exposure: plugin_mgr.ExposureSpec{
			Channels: []plugin_mgr.ExposureChannel{
				{
					Type:       "rest",
					Method:     "POST",
					Entrypoint: "${POWERX_PLUGIN_HTTP_BASE:-/api/v1}/integration/acme/webhooks/shopify",
					Auth:       "public",
					Security:   map[string]any{"verifier": "shopify_hmac"},
				},
			},
		},
	}

	pol := PolicyFromPlugin(p)
	if pol == nil {
		t.Fatalf("PolicyFromPlugin returned nil")
	}
	if len(pol.PublicRoutes) != 1 {
		t.Fatalf("PublicRoutes len=%d, want 1", len(pol.PublicRoutes))
	}
	got := pol.PublicRoutes[0]
	if got.Method != "POST" || got.Path != "/api/v1/integration/acme/webhooks/shopify" {
		t.Fatalf("unexpected public route: %+v", got)
	}
}

func TestPolicyFromPlugin_AddProtectedExposureRoutes(t *testing.T) {
	t.Parallel()

	p := plugin_mgr.Plugin{
		Endpoints: plugin_mgr.EndpointSpec{HTTPBasePath: "/api/v1"},
		Exposure: plugin_mgr.ExposureSpec{
			Channels: []plugin_mgr.ExposureChannel{
				{
					Type:       "rest",
					Method:     "GET",
					Entrypoint: "${POWERX_PLUGIN_HTTP_BASE:-/api/v1}/admin/social/channel-accounts/{account_uuid}",
					Auth:       "jwt",
					RBAC:       "scrm.social_channel_accounts:read",
				},
			},
		},
	}

	pol := PolicyFromPlugin(p)
	if pol == nil {
		t.Fatalf("PolicyFromPlugin returned nil")
	}
	perm, ok := pol.Routes["GET:/api/v1/admin/social/channel-accounts/*"]
	if !ok {
		t.Fatalf("protected exposure route not installed: %+v", pol.Routes)
	}
	if perm.Resource != "scrm.social_channel_accounts" || perm.Action != "read" {
		t.Fatalf("permission = %+v, want scrm.social_channel_accounts:read", perm)
	}
}
