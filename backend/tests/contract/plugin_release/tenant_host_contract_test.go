package plugin_release

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTenantPluginReleaseHostContractUsesServiceActorUUIDRoutes(t *testing.T) {
	root := tenantPluginReleaseRepoRoot(t)
	spec, err := os.ReadFile(filepath.Join(root, "specs", "009-install-plugin-pxp", "contracts", "http-openapi.yaml"))
	require.NoError(t, err)
	content := string(spec)
	for _, required := range []string{
		"/tenant/plugin-release/install-sessions", "/tenant/plugin-release/install-sessions/{session_uuid}",
		"/tenant/plugin-release/install-sessions/{session_uuid}/stop", "/tenant/plugin-release/import-jobs/{job_uuid}",
		"PLUGIN_RELEASE_UNAUTHORIZED", "PLUGIN_RELEASE_FORBIDDEN", "PLUGIN_RELEASE_ACTIVE_SESSION_CONFLICT",
		"PLUGIN_RELEASE_PACKAGE_VERIFICATION_FAILED", "PLUGIN_RELEASE_UPSTREAM_DEPENDENCY",
	} {
		require.Contains(t, content, required)
	}

	routes, err := os.ReadFile(filepath.Join(root, "backend", "internal", "transport", "http", "openapi", "plugin_release", "routes.go"))
	require.NoError(t, err)
	require.Contains(t, string(routes), `routes.POST("/install-sessions", handler.startSession)`)
	require.NotContains(t, string(routes), "developer_member_uuid")
}

func tenantPluginReleaseRepoRoot(t testing.TB) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "..", ".."))
}
