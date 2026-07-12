package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"gopkg.in/yaml.v3"
)

func TestInjectGlobalGatewayRuntimeEnvInjectsRuntimeSTSAndRemovesDeprecatedCredentials(t *testing.T) {
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
			RuntimeCredential: func(ctx context.Context, pluginID string) (*PluginRuntimeCredential, error) {
				if pluginID != "com.powerx.plugins.demo" {
					t.Fatalf("pluginID = %q", pluginID)
				}
				return &PluginRuntimeCredential{
					TenantUUID:     "00000000-0000-0000-0000-000000000001",
					ClientID:       "com.powerx.plugins.demo.00000000-0000-0000-0000-000000000001",
					ClientSecret:   "runtime-secret",
					GRPCAddress:    "127.0.0.1:9001",
					STSAudience:    "powerx:api",
					STSScope:       "access",
					GatewayBaseURL: "http://127.0.0.1:8077",
				}, nil
			},
		},
	}

	env := map[string]string{
		"PX_GATEWAY_API_KEY":     "legacy-api-key",
		"PX_GATEWAY_TENANT_UUID": "6b5d0240-9920-46da-b707-88200e0f51ea",
		"PX_PLUGIN_TOOL_TOKEN":   "stale-tool-token",
		"PX_TOOL_TOKEN":          "legacy-tool-token",
	}
	if err := m.injectGlobalGatewayRuntimeEnv(env, "com.powerx.plugins.demo", nil); err != nil {
		t.Fatalf("injectGlobalGatewayRuntimeEnv() err = %v", err)
	}

	if got := env["PX_GATEWAY_BASE_URL"]; got != "http://127.0.0.1:8077" {
		t.Fatalf("PX_GATEWAY_BASE_URL = %q, want %q", got, "http://127.0.0.1:8077")
	}
	if got := env["PX_GATEWAY_AUTH_SCHEME"]; got != "bearer" {
		t.Fatalf("PX_GATEWAY_AUTH_SCHEME = %q, want bearer", got)
	}
	if got := env["POWERX_STS_CLIENT_ID"]; got != "com.powerx.plugins.demo.00000000-0000-0000-0000-000000000001" {
		t.Fatalf("POWERX_STS_CLIENT_ID = %q", got)
	}
	if got := env["POWERX_STS_CLIENT_SECRET"]; got != "runtime-secret" {
		t.Fatalf("POWERX_STS_CLIENT_SECRET = %q", got)
	}
	if got := env["POWERX_GRPC_UPSTREAM_ADDRESS"]; got != "127.0.0.1:9001" {
		t.Fatalf("POWERX_GRPC_UPSTREAM_ADDRESS = %q", got)
	}
	if got := env["POWERX_GRPC_UPSTREAM_TENANT_UUID"]; got != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("POWERX_GRPC_UPSTREAM_TENANT_UUID = %q", got)
	}
	for _, key := range []string{
		"PX_GATEWAY_TENANT_UUID",
		"PX_PLUGIN_TOOL_TOKEN",
		"PX_GATEWAY_API_KEY",
		"PX_TOOL_TOKEN",
	} {
		if _, ok := env[key]; ok {
			t.Fatalf("%s should not be set on global plugin runtime process", key)
		}
	}
}

func TestDelegatedHostContractPrefersRuntimeEnvOverStaleHostValues(t *testing.T) {
	t.Setenv("POWERX_HTTP_PROXY_BASE", "http://127.0.0.1:8081/")
	t.Setenv("POWERX_PUBLIC_BASE_URL", "https://agent-dev.example.com/")
	t.Setenv("POWERX_PUBLIC_WS_ORIGIN", "wss://agent-dev.example.com/")

	m := &managerImpl{
		opts: Options{
			CoreConfig: &config.Config{
				Server: config.ServerConfig{Port: 8081},
			},
		},
	}
	env := map[string]string{
		"PX_GATEWAY_BASE_URL":          "http://127.0.0.1:8080",
		"NUXT_PUBLIC_POWERX_CORE_BASE": "https://agent.example.com",
		"NUXT_PUBLIC_WS_ORIGIN":        "ws://127.0.0.1:8080",
		"NUXT_PUBLIC_WS_PATH":          "/api/ws",
		"POWERX_PLUGIN_INSTALLED_ROOT": "/opt/powerx/plugins/installed",
		"POWERX_PLUGIN_REGISTRY_FILE":  "/opt/powerx/plugins/registry.json",
		"POWERX_PLUGIN_CONFIG_DIR":     "/opt/powerx/plugins/installed/com.powerx.plugins.demo/config",
		"POWERX_GRPC_UPSTREAM_ADDRESS": "127.0.0.1:9010",
		"POWERX_DB_DATABASE":           "powerx",
		"POWERX_SERVER_PORT":           "8080",
	}

	m.applyDelegatedHostContract(env, map[string]any{}, "com.powerx.plugins.demo", nil)

	if got := env["PX_GATEWAY_BASE_URL"]; got != "http://127.0.0.1:8081" {
		t.Fatalf("PX_GATEWAY_BASE_URL = %q, want dev internal backend", got)
	}
	if got := env["NUXT_PUBLIC_POWERX_CORE_BASE"]; got != "https://agent-dev.example.com" {
		t.Fatalf("NUXT_PUBLIC_POWERX_CORE_BASE = %q, want dev public base", got)
	}
	if got := env["NUXT_PUBLIC_WS_ORIGIN"]; got != "wss://agent-dev.example.com" {
		t.Fatalf("NUXT_PUBLIC_WS_ORIGIN = %q, want dev public ws origin", got)
	}
}

func TestEnsureDelegatedHostContractForEnableWritesRuntimeSTSToHostValues(t *testing.T) {
	dir := t.TempDir()
	hostValuesPath := filepath.Join(dir, "host-values.yaml")
	initial := []byte(`
env:
  PX_GATEWAY_BASE_URL: http://127.0.0.1:8077
  PX_GATEWAY_AUTH_SCHEME: bearer
`)
	if err := os.WriteFile(hostValuesPath, initial, 0o640); err != nil {
		t.Fatal(err)
	}

	m := &managerImpl{
		opts: Options{
			CoreConfig: &config.Config{
				Server: config.ServerConfig{Port: 8077},
			},
		},
	}
	plugin := plugin_mgr.Plugin{
		ID: "com.powerx.plugins.demo",
		Paths: plugin_mgr.InstalledPaths{
			HostValuesFile: hostValuesPath,
		},
	}
	cred := &PluginRuntimeCredential{
		TenantUUID:     "00000000-0000-0000-0000-000000000001",
		ClientID:       "com.powerx.plugins.demo.00000000-0000-0000-0000-000000000001",
		ClientSecret:   "runtime-secret",
		GRPCAddress:    "127.0.0.1:9001",
		STSAudience:    "powerx:api",
		STSScope:       "access",
		GatewayBaseURL: "http://127.0.0.1:8077",
	}

	if err := m.ensureDelegatedHostContractForEnable(&plugin, cred); err != nil {
		t.Fatalf("ensureDelegatedHostContractForEnable() err = %v", err)
	}

	raw, err := os.ReadFile(hostValuesPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	env, ok := doc["env"].(map[string]any)
	if !ok {
		t.Fatalf("host-values env missing: %#v", doc)
	}
	if got := env["POWERX_STS_CLIENT_ID"]; got != cred.ClientID {
		t.Fatalf("POWERX_STS_CLIENT_ID = %q, want %q", got, cred.ClientID)
	}
	if got := env["POWERX_STS_CLIENT_SECRET"]; got != cred.ClientSecret {
		t.Fatalf("POWERX_STS_CLIENT_SECRET = %q, want %q", got, cred.ClientSecret)
	}
	if got := env["POWERX_GRPC_UPSTREAM_ADDRESS"]; got != cred.GRPCAddress {
		t.Fatalf("POWERX_GRPC_UPSTREAM_ADDRESS = %q, want %q", got, cred.GRPCAddress)
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
