package config

import "testing"

func TestResolveEffectivePortsUsesConfigWebAdminPort(t *testing.T) {
	t.Setenv("POWERX_ENV", "")
	t.Setenv("POWERX_BACKEND_PORT", "")
	t.Setenv("POWERX_WEB_ADMIN_PORT", "")

	ports := ResolveEffectivePorts(&Config{
		Server:       ServerConfig{Port: 18080},
		WebAdminPort: 13000,
	})

	if ports.BackendPort != 18080 {
		t.Fatalf("backend port = %d, want 18080", ports.BackendPort)
	}
	if ports.WebAdminPort != 13000 {
		t.Fatalf("web admin port = %d, want 13000", ports.WebAdminPort)
	}
}

func TestResolveEffectivePortsEnvOverridesConfigWebAdminPort(t *testing.T) {
	t.Setenv("POWERX_ENV", "")
	t.Setenv("POWERX_BACKEND_PORT", "")
	t.Setenv("POWERX_WEB_ADMIN_PORT", "13088")

	ports := ResolveEffectivePorts(&Config{
		WebAdminPort: 13000,
	})

	if ports.WebAdminPort != 13088 {
		t.Fatalf("web admin port = %d, want 13088", ports.WebAdminPort)
	}
}
