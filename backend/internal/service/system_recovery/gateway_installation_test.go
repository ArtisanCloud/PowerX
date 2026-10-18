package system_recovery

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/stretchr/testify/require"
)

func TestFixedVerificationGatewayServesWebAndCoreSocketRoutes(t *testing.T) {
	root, err := os.MkdirTemp("/private/tmp", "px-gateway-")
	require.NoError(t, err)
	defer os.RemoveAll(root)
	serve := func(name, body string) {
		listener, err := net.Listen("unix", filepath.Join(root, name))
		require.NoError(t, err)
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body + ":" + r.URL.Path)) })}
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	}
	serve("core.sock", "core")
	serve("web.sock", "web")
	d := Deployment{Root: root, ConfigFile: filepath.Join(root, "etc/config.yaml")}
	cfg := config.GetDefaults()
	cfg.Server.APIPrefix = "/custom/v3"
	cfg.Server.WSPrefix = "/custom-ws"
	cfg.Plugin.BasePrefix = "/_p"
	cfg.WebAdminPort = 1
	require.NoError(t, writeInstallationConfig(d.ConfigFile, cfg))
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(root, "run.json"), RunSpec{Deployment: d, Isolated: true, VerificationServer: true}))
	for path, body := range map[string]string{"/users/login": "web", "/custom/v3/admin/customers/accounts": "core", "/api/ws": "core", "/custom-ws/agents/stream": "core", "/media/fixture.txt": "core", "/_p/fixture/index": "core"} {
		response := httptest.NewRecorder()
		proxyDeployment(response, httptest.NewRequest("GET", path, nil), d, false)
		require.Equal(t, 200, response.Code, path)
		require.Equal(t, body+":"+path, response.Body.String())
	}
}
