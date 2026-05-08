package audit

import "testing"

func TestExtractPluginIDFromResourceID(t *testing.T) {
	tests := []struct {
		in     string
		expect string
	}{
		{in: "POST /api/v1/integration/demo-plugin/webhooks/shopify", expect: "com.powerx.plugins.demo-plugin"},
		{in: "GET /_p/com.powerx.plugins.demo-plugin/api/v1/admin/x", expect: "com.powerx.plugins.demo-plugin"},
		{in: "GET /api/v2/integration/demo-plugin/test", expect: "com.powerx.plugins.demo-plugin"},
		{in: "GET /api/v1/admin/users", expect: ""},
	}
	for _, tc := range tests {
		got := extractPluginIDFromResourceID(tc.in)
		if got != tc.expect {
			t.Fatalf("unexpected plugin id for %q: got=%q expect=%q", tc.in, got, tc.expect)
		}
	}
}

func TestDecodeMeta(t *testing.T) {
	meta := []byte(`{"request_id":"rid-1","trace_id":"tid-1","plugin_id":"com.powerx.plugins.demo-plugin"}`)
	gotMap := decodeMeta(meta)
	if got := gotMap["request_id"]; got != "rid-1" {
		t.Fatalf("unexpected request_id: %q", got)
	}
	if got := gotMap["trace_id"]; got != "tid-1" {
		t.Fatalf("unexpected trace_id: %q", got)
	}
	if got := gotMap["plugin_id"]; got != "com.powerx.plugins.demo-plugin" {
		t.Fatalf("unexpected plugin_id: %q", got)
	}
}
