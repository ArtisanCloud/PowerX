package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
)

func TestInjectGatewaySecurityEnvSuccess(t *testing.T) {
	t.Setenv("POWERX_GATEWAY_BASE_URL", "http://127.0.0.1:8077/")
	m := &managerImpl{
		opts: Options{
			CoreConfig: &config.Config{
				Server: config.ServerConfig{Port: 8077},
				Auth: config.AuthConfig{
					JWTSecret:    "0123456789abcdef0123456789abcdef",
					Issuer:       "powerx-auth",
					AudienceUser: "user",
					AccessTTLStr: "15m",
				},
			},
		},
	}

	env := map[string]string{
		"PX_GATEWAY_API_KEY": "legacy-api-key",
		"PX_TOOL_TOKEN":      "legacy-tool-token",
	}
	if err := m.injectGatewaySecurityEnv(env, "com.powerx.plugins.demo"); err != nil {
		t.Fatalf("injectGatewaySecurityEnv() err = %v", err)
	}

	if got := env["PX_GATEWAY_BASE_URL"]; got != "http://127.0.0.1:8077" {
		t.Fatalf("PX_GATEWAY_BASE_URL = %q, want %q", got, "http://127.0.0.1:8077")
	}
	if got := env["PX_GATEWAY_AUTH_SCHEME"]; got != "bearer" {
		t.Fatalf("PX_GATEWAY_AUTH_SCHEME = %q, want bearer", got)
	}
	if strings.TrimSpace(env["PX_PLUGIN_TOOL_TOKEN"]) == "" {
		t.Fatalf("PX_PLUGIN_TOOL_TOKEN should be generated")
	}
	if _, ok := env["PX_GATEWAY_API_KEY"]; ok {
		t.Fatalf("PX_GATEWAY_API_KEY should be removed in delegated contract")
	}
	if _, ok := env["PX_TOOL_TOKEN"]; ok {
		t.Fatalf("PX_TOOL_TOKEN should be removed in delegated contract")
	}
}

func TestInjectGatewaySecurityEnvFailFastMissingBaseURL(t *testing.T) {
	t.Setenv("POWERX_GATEWAY_BASE_URL", "")
	m := &managerImpl{
		opts: Options{
			CoreConfig: &config.Config{
				Server: config.ServerConfig{Port: 0},
				Auth: config.AuthConfig{
					JWTSecret:    "0123456789abcdef0123456789abcdef",
					Issuer:       "powerx-auth",
					AudienceUser: "user",
				},
			},
		},
	}

	err := m.injectGatewaySecurityEnv(map[string]string{}, "com.powerx.plugins.demo")
	if err == nil {
		t.Fatalf("injectGatewaySecurityEnv() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "GW_CFG_MISSING_BASE_URL") {
		t.Fatalf("injectGatewaySecurityEnv() err = %v, want GW_CFG_MISSING_BASE_URL", err)
	}
}

func TestInjectGatewaySecurityEnvFailFastInvalidAuthScheme(t *testing.T) {
	t.Setenv("POWERX_GATEWAY_BASE_URL", "http://127.0.0.1:8077")
	m := &managerImpl{
		opts: Options{
			CoreConfig: &config.Config{
				Server: config.ServerConfig{Port: 8077},
				Auth: config.AuthConfig{
					JWTSecret:    "0123456789abcdef0123456789abcdef",
					Issuer:       "",
					AudienceUser: "user",
				},
			},
		},
	}

	err := m.injectGatewaySecurityEnv(map[string]string{}, "com.powerx.plugins.demo")
	if err == nil {
		t.Fatalf("injectGatewaySecurityEnv() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "GW_CFG_INVALID_AUTH_SCHEME") {
		t.Fatalf("injectGatewaySecurityEnv() err = %v, want GW_CFG_INVALID_AUTH_SCHEME", err)
	}
}

func TestProbeGatewayContractSuccess(t *testing.T) {
	var gotAuth string
	var gotTenant string

	gatewaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tenant/capabilities" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		gotTenant = r.Header.Get("tenant_uuid")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	defer gatewaySrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer apiSrv.Close()

	m := &managerImpl{}
	env := map[string]string{
		"PX_GATEWAY_BASE_URL":  gatewaySrv.URL,
		"PX_PLUGIN_TOOL_TOKEN": "tool-token-1",
	}
	err := m.probeGatewayContract(context.Background(), "com.powerx.plugins.demo", apiSrv.URL, "/healthz", env, true, "tenant-1")
	if err != nil {
		t.Fatalf("probeGatewayContract() err = %v", err)
	}
	if gotAuth != "Bearer tool-token-1" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer tool-token-1")
	}
	if gotTenant != "tenant-1" {
		t.Fatalf("tenant_uuid header = %q, want %q", gotTenant, "tenant-1")
	}
}

func TestProbeGatewayContractFailFastMissingToken(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer apiSrv.Close()

	m := &managerImpl{}
	env := map[string]string{
		"PX_GATEWAY_BASE_URL": "http://127.0.0.1:8077",
	}
	err := m.probeGatewayContract(context.Background(), "com.powerx.plugins.demo", apiSrv.URL, "/healthz", env, true, "tenant-1")
	if err == nil {
		t.Fatalf("probeGatewayContract() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "GW_CFG_MISSING_PLUGIN_TOOL_TOKEN") {
		t.Fatalf("probeGatewayContract() err = %v, want GW_CFG_MISSING_PLUGIN_TOOL_TOKEN", err)
	}
}
