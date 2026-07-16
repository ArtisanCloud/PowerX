package manager

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBootstrapKeepsManagerAvailableWhenPermissionSyncFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	installedRoot := filepath.Join(root, "installed")
	registry := NewJSONRegistry(filepath.Join(root, "registry.json"))
	pluginRoot := filepath.Join(installedRoot, "com.powerx.plugins.bad-permission", "0.1.0")
	writeBootstrapTestPlugin(t, pluginRoot)

	m := &managerImpl{
		opts: Options{
			Enabled:       true,
			InstalledRoot: installedRoot,
			Registry:      registry,
			Loader:        NewFSLoader(),
			PostInstallManifest: func(ctx context.Context, manifest plugin_mgr.Manifest) error {
				return errors.New("permission resource is too long")
			},
		},
	}

	require.NoError(t, m.Bootstrap(ctx))

	items, err := m.List(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "com.powerx.plugins.bad-permission", items[0].ID)
	require.Equal(t, plugin_mgr.StateInstalled, items[0].State)
}

func TestBootstrapDefersEnabledPluginRestore(t *testing.T) {
	if os.Getenv("POWERX_TEST_PLUGIN_PROCESS") == "1" {
		runBootstrapTestPluginProcess()
		return
	}

	ctx := context.Background()
	root := t.TempDir()
	installedRoot := filepath.Join(root, "installed")
	registry := NewJSONRegistry(filepath.Join(root, "registry.json"))
	require.NoError(t, registry.Load(ctx))
	pluginRoot := filepath.Join(installedRoot, "com.powerx.plugins.restore-test", "0.1.0")
	writeBootstrapRestoreTestPlugin(t, pluginRoot)

	desc, err := NewFSLoader().LoadDescriptor(ctx, pluginRoot)
	require.NoError(t, err)
	require.NoError(t, registry.Put(ctx, desc, plugin_mgr.StateEnabled))
	require.NoError(t, registry.UpdateState(ctx, desc.Manifest.ID, desc.Manifest.Version, plugin_mgr.StateEnabled))
	require.NoError(t, registry.Save(ctx))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	dr := router.NewDynamicRouter("/_p", engine)
	m := &managerImpl{
		opts: Options{
			Enabled:       true,
			InstalledRoot: installedRoot,
			Registry:      registry,
			Loader:        NewFSLoader(),
			HTTP:          dr,
			Supervisor:    supervisor.New(),
			CoreConfig: &config.Config{
				Server: config.ServerConfig{Port: 8077},
			},
			RuntimeCredential: func(ctx context.Context, pluginID string) (*PluginRuntimeCredential, error) {
				require.Equal(t, "com.powerx.plugins.restore-test", pluginID)
				return &PluginRuntimeCredential{
					TenantUUID:     "00000000-0000-0000-0000-000000000001",
					ClientID:       "com.powerx.plugins.restore-test.00000000-0000-0000-0000-000000000001",
					ClientSecret:   "runtime-secret",
					GRPCAddress:    "127.0.0.1:9001",
					STSAudience:    "powerx:api",
					STSScope:       "access",
					GatewayBaseURL: "http://127.0.0.1:8077",
				}, nil
			},
		},
		http: dr,
		sup:  supervisor.New(),
	}
	m.sup = m.opts.Supervisor

	require.NoError(t, m.Bootstrap(ctx))

	items, err := m.List(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, plugin_mgr.StateEnabled, items[0].State)

	resp := performBootstrapDebugRequest(engine)
	require.NotContains(t, resp, `"com.powerx.plugins.restore-test"`)

	require.NoError(t, m.Enable(ctx, "com.powerx.plugins.restore-test"))
	t.Cleanup(func() {
		_ = m.Disable(ctx, "com.powerx.plugins.restore-test")
	})

	resp = performBootstrapDebugRequest(engine)
	require.Contains(t, resp, `"com.powerx.plugins.restore-test"`)
	require.Contains(t, resp, `"basePath":"/api/v1"`)
}

func TestMountDebugHostUsesExactLocalPluginID(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	dr := router.NewDynamicRouter("/_p", engine)
	ctx := context.Background()
	registry := NewJSONRegistry(filepath.Join(t.TempDir(), "registry.json"))
	require.NoError(t, registry.Put(ctx, Descriptor{
		Manifest: plugin_mgr.Manifest{
			ID:      "com.powerx.plugins.base",
			Version: "0.1.3",
			Endpoints: plugin_mgr.EndpointSpec{
				HTTPBasePath: "/api/v1",
			},
		},
	}, plugin_mgr.StateEnabled))
	m := &managerImpl{opts: Options{Registry: registry}, http: dr}

	require.NoError(t, MountDebugHost(m, "com.powerx.plugins.base.local", 3131))

	resp := performBootstrapDebugRequest(engine)
	require.Contains(t, resp, `"com.powerx.plugins.base.local"`)
	require.NotContains(t, resp, `"com.powerx.plugins.base":`)
	require.Contains(t, resp, `"basePath":"/api/v1"`)
}

func TestDebugHostPolicyUsesSourcePluginManifestForLocalPlugin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := NewJSONRegistry(filepath.Join(t.TempDir(), "registry.json"))
	require.NoError(t, registry.Put(ctx, Descriptor{
		Manifest: plugin_mgr.Manifest{
			ID:      "com.powerx.plugins.base",
			Version: "0.1.3",
			Endpoints: plugin_mgr.EndpointSpec{
				HTTPBasePath: "/api/v1",
			},
			Exposure: plugin_mgr.ExposureSpec{
				Channels: []plugin_mgr.ExposureChannel{
					{
						Type:       "rest",
						Method:     http.MethodPost,
						Entrypoint: "/api/v1/templates",
						Auth:       "jwt",
						RBAC:       "template:create",
					},
				},
			},
			RBAC: plugin_mgr.RBACSpec{
				Resources: []plugin_mgr.RBACResource{
					{Resource: "template", Actions: []string{"create"}},
				},
			},
		},
	}, plugin_mgr.StateEnabled))

	policy, err := debugHostPolicyForPlugin(&managerImpl{opts: Options{Registry: registry}}, "com.powerx.plugins.base.local")
	require.NoError(t, err)
	require.NotNil(t, policy)
	require.Equal(t, "/api/v1", policy.HTTPBase)
	require.Equal(t, &router.Permission{Resource: "template", Action: "create"}, policy.Required(http.MethodPost, "/api/v1/templates"))
}

func performBootstrapDebugRequest(engine *gin.Engine) string {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/__debug/plugins", nil)
	engine.ServeHTTP(rec, req)
	return rec.Body.String()
}

func runBootstrapTestPluginProcess() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
	addr := os.Getenv("POWERX_HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	if err := http.ListenAndServe(addr, mux); err != nil {
		os.Exit(1)
	}
}

func writeBootstrapTestPlugin(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "web-admin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(`
id: com.powerx.plugins.bad-permission
name: Bad Permission
version: 0.1.0
runtime:
  kind: static
endpoints:
  http_base_path: /api/v1
routes:
  basePath: /api/v1
frontend:
  admin:
    kind: static
    static_dir: web-admin
`), 0o644))
}

func writeBootstrapRestoreTestPlugin(t *testing.T, root string) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "bin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "web-admin"), 0o755))
	wrapper := "#!/bin/sh\nPOWERX_TEST_PLUGIN_PROCESS=1 exec " + exe + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "backend", "bin", "plugin"), []byte(wrapper), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(`
id: com.powerx.plugins.restore-test
name: Restore Test
version: 0.1.0
runtime:
  kind: process
  entry: backend/bin/plugin
  health:
    http: /healthz
    interval: 100ms
    timeout: 100ms
endpoints:
  http_base_path: /api/v1
frontend:
  admin:
    kind: static
    static_dir: web-admin
`), 0o644))
}
