package plugincontract

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestPluginLifecycleBoundaryHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHereLifecycle(t), "specs", "026-iam", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load 026-iam openapi spec")

	enablePath, ok := doc.Paths["/admin/plugins/tenant-instances/{plugin_id}/enable"]
	require.True(t, ok, "tenant enable path must exist")
	require.NotNil(t, enablePath.Post)
	require.NotEqual(t, "enablePluginPackage", enablePath.Post.OperationID, "tenant enable must not be package lifecycle enable")

	disablePath, ok := doc.Paths["/admin/plugins/tenant-instances/{plugin_id}/disable"]
	require.True(t, ok, "tenant disable path must exist")
	require.NotNil(t, disablePath.Post)
	require.NotEqual(t, "disablePluginPackage", disablePath.Post.OperationID, "tenant disable must not be package lifecycle disable")

	uninstallPath, ok := doc.Paths["/admin/plugins/{plugin_id}/uninstall"]
	require.True(t, ok, "global plugin uninstall contract must exist")
	require.NotNil(t, uninstallPath.Post)
	require.Contains(t, uninstallPath.Post.Responses, "409", "uninstall must report tenant instance impact conflict")
}

func repoRootFromHereLifecycle(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve current file path")
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
