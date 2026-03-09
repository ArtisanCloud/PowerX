package router

import "testing"

func TestIsIdentityAuthClientPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "v1 user auth me", path: "/v1/admin/user/auth/me/context", want: true},
		{name: "v1 supply auth login", path: "/v1/admin/supply/auth/login", want: true},
		{name: "api v1 user auth", path: "/api/v1/admin/user/auth/refresh", want: true},
		{name: "future identity auth", path: "/v1/admin/channel_partner/auth/logout", want: true},
		{name: "legacy admin auth", path: "/v1/admin/auth/me/context", want: false},
		{name: "normal plugin api", path: "/v1/templates?page=1&page_size=50", want: false},
		{name: "admin non-auth route", path: "/v1/admin/user/profile", want: false},
		{name: "too short", path: "/v1/admin/user", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isIdentityAuthClientPath(tc.path)
			if got != tc.want {
				t.Fatalf("isIdentityAuthClientPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestToHostIdentityAuthPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "keep api v1", in: "/api/v1/admin/user/auth/me/context", want: "/api/v1/admin/user/auth/me/context"},
		{name: "v1 to api", in: "/v1/admin/user/auth/me/context", want: "/api/v1/admin/user/auth/me/context"},
		{name: "admin direct to api v1", in: "/admin/supply/auth/login", want: "/api/v1/admin/supply/auth/login"},
		{name: "no leading slash", in: "v1/admin/user/auth/refresh", want: "/api/v1/admin/user/auth/refresh"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := toHostIdentityAuthPath(tc.in)
			if got != tc.want {
				t.Fatalf("toHostIdentityAuthPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
