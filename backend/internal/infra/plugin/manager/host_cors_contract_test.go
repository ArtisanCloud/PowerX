package manager

import (
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
)

func TestHostCORSOriginsPreferWebAdminOrigins(t *testing.T) {
	t.Setenv("POWERX_WEB_ADMIN_ORIGINS", "")
	t.Setenv("POWERX_PLUGIN_CORS_ORIGINS", "")

	m := &managerImpl{
		opts: Options{
			CoreConfig: &config.Config{
				WebAdminPort: 13000,
				HTTPSecurity: config.HTTPSecurityConfig{
					WebAdminOrigins: []string{"https://admin.example.com"},
					FrameAncestors:  []string{"'self'", "https://frame.example.com"},
				},
			},
		},
	}

	got := m.hostCORSOrigins()
	want := []string{
		"https://admin.example.com",
		"https://frame.example.com",
		"http://localhost:13000",
		"http://127.0.0.1:13000",
	}
	if len(got) != len(want) {
		t.Fatalf("origins = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("origins[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestHostCORSOriginsReadsWebAdminOriginsEnv(t *testing.T) {
	t.Setenv("POWERX_WEB_ADMIN_ORIGINS", "https://admin.example.com,https://console.example.com")
	t.Setenv("POWERX_PLUGIN_CORS_ORIGINS", "")

	m := &managerImpl{}

	got := m.hostCORSOrigins()
	want := []string{"https://admin.example.com", "https://console.example.com"}
	if len(got) != len(want) {
		t.Fatalf("origins = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("origins[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}
