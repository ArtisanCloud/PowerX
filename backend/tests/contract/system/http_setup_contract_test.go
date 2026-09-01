package systemcontract

import (
	"net/http"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestSetupHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "025-powerx-docker-systemd", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load setup openapi spec")

	assertOperation(t, doc, "/admin/setup/status", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/admin/setup/config", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/admin/setup/config", http.MethodPut, []string{"200"})
	assertOperation(t, doc, "/admin/setup/complete", http.MethodPost, []string{"200"})

	schema := doc.Components.Schemas["SetupConfig"]
	require.NotNil(t, schema, "SetupConfig schema missing")
	require.Contains(t, schema.Value.Required, "deployment", "SetupConfig.deployment must be required")
	require.Contains(t, schema.Value.Properties, "deployment", "SetupConfig.deployment schema missing")
	deployment := schema.Value.Properties["deployment"]
	require.NotNil(t, deployment.Value)
	require.Contains(t, deployment.Value.Required, "env", "SetupConfig.deployment.env must be required")
	require.Equal(t, []any{"dev", "test", "staging", "prod"}, deployment.Value.Properties["env"].Value.Enum)
	require.Contains(t, schema.Value.Properties, "ports", "SetupConfig.ports schema missing")
}

func assertOperation(t testing.TB, doc *openapi3.T, path, method string, statuses []string) {
	t.Helper()
	item, ok := doc.Paths[path]
	require.True(t, ok, "path %s missing", path)
	op := getOperation(item, method)
	require.NotNil(t, op, "operation %s %s missing", method, path)
	for _, code := range statuses {
		_, ok := op.Responses[code]
		require.True(t, ok, "status %s missing on %s %s", code, method, path)
	}
}

func getOperation(item *openapi3.PathItem, method string) *openapi3.Operation {
	switch method {
	case http.MethodGet:
		return item.Get
	case http.MethodPost:
		return item.Post
	case http.MethodPut:
		return item.Put
	default:
		return nil
	}
}

func repoRootFromHere(t testing.TB) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime caller failed")
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "..")
}
