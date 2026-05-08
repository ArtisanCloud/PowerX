package reqctx

import "testing"

func TestResolvePluginIDFromPathWithAPIPrefix(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		apiPrefix string
		expect    string
	}{
		{
			name:      "_p route",
			path:      "/_p/com.powerx.plugins.demo-plugin/api/v1/admin/x",
			apiPrefix: "/api",
			expect:    "com.powerx.plugins.demo-plugin",
		},
		{
			name:      "integration v1 route",
			path:      "/api/v1/integration/demo-plugin/webhooks/shopify",
			apiPrefix: "/api",
			expect:    "com.powerx.plugins.demo-plugin",
		},
		{
			name:      "integration v2 route",
			path:      "/api/v2/integration/demo-plugin/webhooks/shopify",
			apiPrefix: "/api",
			expect:    "com.powerx.plugins.demo-plugin",
		},
		{
			name:      "custom api prefix direct integration",
			path:      "/openapi/integration/demo-plugin/webhooks/shopify",
			apiPrefix: "/openapi",
			expect:    "com.powerx.plugins.demo-plugin",
		},
		{
			name:      "already full plugin id",
			path:      "/api/v9/integration/com.powerx.plugins.demo-plugin/webhooks/shopify",
			apiPrefix: "/api",
			expect:    "com.powerx.plugins.demo-plugin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolvePluginIDFromPathWithAPIPrefix(tc.path, tc.apiPrefix)
			if got != tc.expect {
				t.Fatalf("unexpected plugin id, got=%q expect=%q", got, tc.expect)
			}
		})
	}
}

func TestResolvePluginIDFromPath_GenericPrefixFallback(t *testing.T) {
	got := ResolvePluginIDFromPath("/openapi/integration/demo-plugin/webhooks/shopify")
	expect := "com.powerx.plugins.demo-plugin"
	if got != expect {
		t.Fatalf("unexpected plugin id, got=%q expect=%q", got, expect)
	}
}
