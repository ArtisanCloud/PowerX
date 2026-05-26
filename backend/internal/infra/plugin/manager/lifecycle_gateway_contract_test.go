package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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
		"PX_GATEWAY_API_KEY":               "legacy-api-key",
		"PX_PLUGIN_TOOL_TOKEN":             "stale-tool-token",
		"PX_TOOL_TOKEN":                    "legacy-tool-token",
		"POWERX_STS_CLIENT_ID":             "com.powerx.plugins.demo.tenant",
		"POWERX_STS_CLIENT_SECRET":         "secret",
		"POWERX_GRPC_UPSTREAM_ADDRESS":     "127.0.0.1:9001",
		"POWERX_GRPC_UPSTREAM_TENANT_UUID": "6b5d0240-9920-46da-b707-88200e0f51ea",
	}
	ctx := reqctx.WithUserID(context.Background(), 1)
	ctx = reqctx.WithMemberID(ctx, 1)
	ctx = reqctx.WithSubject(ctx, "fda3589b-d30b-41b0-a859-c061c179fb58")
	if err := m.injectGatewaySecurityEnv(ctx, env, "com.powerx.plugins.demo", "6b5d0240-9920-46da-b707-88200e0f51ea"); err != nil {
		t.Fatalf("injectGatewaySecurityEnv() err = %v", err)
	}

	if got := env["PX_GATEWAY_BASE_URL"]; got != "http://127.0.0.1:8077" {
		t.Fatalf("PX_GATEWAY_BASE_URL = %q, want %q", got, "http://127.0.0.1:8077")
	}
	if got := env["PX_GATEWAY_AUTH_SCHEME"]; got != "bearer" {
		t.Fatalf("PX_GATEWAY_AUTH_SCHEME = %q, want bearer", got)
	}
	if got := env["PX_GATEWAY_TENANT_UUID"]; got != "6b5d0240-9920-46da-b707-88200e0f51ea" {
		t.Fatalf("PX_GATEWAY_TENANT_UUID = %q", got)
	}
	if _, ok := env["PX_PLUGIN_TOOL_TOKEN"]; ok {
		t.Fatalf("PX_PLUGIN_TOOL_TOKEN should not be set in delegated STS contract")
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

	ctx := reqctx.WithUserID(context.Background(), 1)
	err := m.injectGatewaySecurityEnv(ctx, map[string]string{}, "com.powerx.plugins.demo", "6b5d0240-9920-46da-b707-88200e0f51ea")
	if err == nil {
		t.Fatalf("injectGatewaySecurityEnv() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "GW_CFG_MISSING_BASE_URL") {
		t.Fatalf("injectGatewaySecurityEnv() err = %v, want GW_CFG_MISSING_BASE_URL", err)
	}
}

func TestInjectGatewaySecurityEnvFailFastMissingSTSClient(t *testing.T) {
	t.Setenv("POWERX_GATEWAY_BASE_URL", "http://127.0.0.1:8077")
	m := &managerImpl{
		opts: Options{
			CoreConfig: &config.Config{
				Server: config.ServerConfig{Port: 8077},
				Auth: config.AuthConfig{
					JWTSecret: "0123456789abcdef0123456789abcdef",
				},
			},
		},
	}

	ctx := reqctx.WithUserID(context.Background(), 1)
	err := m.injectGatewaySecurityEnv(ctx, map[string]string{}, "com.powerx.plugins.demo", "6b5d0240-9920-46da-b707-88200e0f51ea")
	if err == nil {
		t.Fatalf("injectGatewaySecurityEnv() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "STS_BOOTSTRAP_CONTRACT_BROKEN") {
		t.Fatalf("injectGatewaySecurityEnv() err = %v, want STS_BOOTSTRAP_CONTRACT_BROKEN", err)
	}
}

func TestInjectGatewayBootstrapEnvDoesNotMintToolToken(t *testing.T) {
	t.Setenv("POWERX_GATEWAY_BASE_URL", "http://127.0.0.1:8077/")
	t.Setenv("POWERX_GATEWAY_BOOTSTRAP_TENANT_UUID", "6b5d0240-9920-46da-b707-88200e0f51ea")
	m := &managerImpl{
		opts: Options{
			CoreConfig: &config.Config{
				Server: config.ServerConfig{Port: 8077},
			},
		},
	}

	env := map[string]string{
		"PX_PLUGIN_TOOL_TOKEN": "stale-token",
		"PX_GATEWAY_API_KEY":   "legacy-api-key",
		"PX_TOOL_TOKEN":        "legacy-tool-token",
	}
	if err := m.injectGatewayBootstrapEnv(env, "com.powerx.plugins.demo"); err != nil {
		t.Fatalf("injectGatewayBootstrapEnv() err = %v", err)
	}

	if got := env["PX_GATEWAY_BASE_URL"]; got != "http://127.0.0.1:8077" {
		t.Fatalf("PX_GATEWAY_BASE_URL = %q, want %q", got, "http://127.0.0.1:8077")
	}
	if got := env["PX_GATEWAY_AUTH_SCHEME"]; got != "bearer" {
		t.Fatalf("PX_GATEWAY_AUTH_SCHEME = %q, want bearer", got)
	}
	if got := env["PX_GATEWAY_TENANT_UUID"]; got != "6b5d0240-9920-46da-b707-88200e0f51ea" {
		t.Fatalf("PX_GATEWAY_TENANT_UUID = %q", got)
	}
	if _, ok := env["PX_PLUGIN_TOOL_TOKEN"]; ok {
		t.Fatalf("PX_PLUGIN_TOOL_TOKEN should not be set during bootstrap")
	}
	if _, ok := env["PX_GATEWAY_API_KEY"]; ok {
		t.Fatalf("PX_GATEWAY_API_KEY should be removed in delegated bootstrap contract")
	}
	if _, ok := env["PX_TOOL_TOKEN"]; ok {
		t.Fatalf("PX_TOOL_TOKEN should be removed in delegated bootstrap contract")
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
		"PX_GATEWAY_BASE_URL":            gatewaySrv.URL,
		"PX_GATEWAY_PROBE_AUTH_REQUIRED": "false",
	}
	err := m.probeGatewayContract(context.Background(), "com.powerx.plugins.demo", apiSrv.URL, "/healthz", env, true, "6b5d0240-9920-46da-b707-88200e0f51ea")
	if err != nil {
		t.Fatalf("probeGatewayContract() err = %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization header = %q, want empty for unauthenticated dry-run", gotAuth)
	}
	if gotTenant != "6b5d0240-9920-46da-b707-88200e0f51ea" {
		t.Fatalf("tenant_uuid header = %q, want %q", gotTenant, "6b5d0240-9920-46da-b707-88200e0f51ea")
	}
}

func TestProbeGatewayContractSkipsBootstrapWhenTokenMissing(t *testing.T) {
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
	err := m.probeGatewayContract(context.Background(), "com.powerx.plugins.demo", apiSrv.URL, "/healthz", env, true, "6b5d0240-9920-46da-b707-88200e0f51ea")
	if err != nil {
		t.Fatalf("probeGatewayContract() err = %v", err)
	}
}

func TestResolveGatewayProbePolicy(t *testing.T) {
	def := resolveGatewayProbePolicy(nil)
	if def.Path != "/api/v1/tenant/capabilities?page_size=1" {
		t.Fatalf("default path = %q", def.Path)
	}
	if !def.AuthRequired || !def.TenantScoped {
		t.Fatalf("default policy should require auth and tenant")
	}

	env := map[string]string{
		"PX_GATEWAY_PROBE_PATH":          "/api/v1/open/ping",
		"PX_GATEWAY_PROBE_AUTH_REQUIRED": "false",
		"PX_GATEWAY_PROBE_TENANT_SCOPED": "0",
	}
	got := resolveGatewayProbePolicy(env)
	if got.Path != "/api/v1/open/ping" {
		t.Fatalf("custom path = %q", got.Path)
	}
	if got.AuthRequired {
		t.Fatalf("auth_required should be false")
	}
	if got.TenantScoped {
		t.Fatalf("tenant_scoped should be false")
	}
}
