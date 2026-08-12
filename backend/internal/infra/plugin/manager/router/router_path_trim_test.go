package router

import "testing"

func TestTrimToAPIClientPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "keep api v1 admin path",
			in:   "/_p/com.powerx.plugins.base/api/v1/admin/runtime/ws-bus/test-flow",
			want: "/api/v1/admin/runtime/ws-bus/test-flow",
		},
		{
			name: "keep api v1 root",
			in:   "/_p/com.powerx.plugins.base/api/v1/templates",
			want: "/api/v1/templates",
		},
		{
			name: "non plugin path passthrough",
			in:   "/api/v1/admin/runtime/ws-bus/test-flow",
			want: "/api/v1/admin/runtime/ws-bus/test-flow",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := trimToAPIClientPath(tc.in)
			if got != tc.want {
				t.Fatalf("trimToAPIClientPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeGatePathForPolicy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		inPath   string
		basePath string
		want     string
	}{
		{
			name:     "v1 path strips api v1 base alias",
			inPath:   "/v1/admin/runtime/ws-bus/test-flow",
			basePath: "/api/v1",
			want:     "/admin/runtime/ws-bus/test-flow",
		},
		{
			name:     "api path strips base",
			inPath:   "/api/v1/admin/runtime/ws-bus/test-flow",
			basePath: "/api/v1",
			want:     "/admin/runtime/ws-bus/test-flow",
		},
		{
			name:     "no base no change",
			inPath:   "/v1/admin/runtime/ws-bus/test-flow",
			basePath: "",
			want:     "/v1/admin/runtime/ws-bus/test-flow",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeGatePathForPolicy(tc.inPath, tc.basePath)
			if got != tc.want {
				t.Fatalf("normalizeGatePathForPolicy(%q,%q)=%q, want %q", tc.inPath, tc.basePath, got, tc.want)
			}
		})
	}
}

func TestBuildAPIUpstreamPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		targetPath string
		basePath   string
		inPath     string
		want       string
	}{
		{
			name:       "avoid duplicated api v1 prefix",
			targetPath: "",
			basePath:   "/api/v1",
			inPath:     "/api/v1/admin/runtime/ws-bus/test-flow",
			want:       "/api/v1/admin/runtime/ws-bus/test-flow",
		},
		{
			name:       "append base when missing",
			targetPath: "",
			basePath:   "/api/v1",
			inPath:     "/admin/runtime/ws-bus/test-flow",
			want:       "/api/v1/admin/runtime/ws-bus/test-flow",
		},
		{
			name:       "respect target path prefix",
			targetPath: "/prefix",
			basePath:   "/api/v1",
			inPath:     "/api/v1/admin/runtime/ws-bus/test-flow",
			want:       "/prefix/api/v1/admin/runtime/ws-bus/test-flow",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildAPIUpstreamPath(tc.targetPath, tc.basePath, tc.inPath)
			if got != tc.want {
				t.Fatalf("buildAPIUpstreamPath(%q,%q,%q)=%q, want %q", tc.targetPath, tc.basePath, tc.inPath, got, tc.want)
			}
		})
	}
}
