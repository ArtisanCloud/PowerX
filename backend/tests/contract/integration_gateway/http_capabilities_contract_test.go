package integrationgatewaycontract

import (
	"net/http"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// TestCapabilityRegistryHTTPContract ensures the HTTP OpenAPI draft for the capability registry
// exposes all endpoints that US1 depends on. This is a lightweight contract test that gives
// early signal when the spec accidentally drops a path/response before the runtime handlers exist.
func TestCapabilityRegistryHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "007-integration-gateway-and-mcp", "contracts", "http-openapi.yaml")

	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "failed to load capability registry OpenAPI: %s", specPath)

	// Basic schema presence sanity checks.
	require.NotNil(t, doc.Paths, "OpenAPI must define paths")
	require.NotNil(t, doc.Components, "OpenAPI must define components")
	schemaRef, ok := doc.Components.Schemas["CapabilityListResponse"]
	require.True(t, ok, "CapabilityListResponse schema missing")
	require.NotNil(t, schemaRef.Value, "CapabilityListResponse schema not expanded")

	assertOperation(t, doc, "/admin/capabilities", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/admin/capabilities/{capabilityId}", http.MethodGet, []string{"200", "404"})
	assertOperation(t, doc, "/admin/capability-sync/jobs", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/tenant/capabilities", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/tenant/invocations", http.MethodPost, []string{"200", "202", "409", "429"})
	assertOperation(t, doc, "/tenant/invocations/{traceId}", http.MethodGet, []string{"200", "404"})
}

func assertOperation(t testing.TB, doc *openapi3.T, path, method string, expectedStatuses []string) {
	t.Helper()
	item, ok := doc.Paths[path]
	require.True(t, ok, "path %s missing from OpenAPI", path)

	op := getOperation(item, method)
	require.NotNil(t, op, "path %s missing %s operation", path, method)
	require.NotNil(t, op.Responses, "operation %s %s lacks responses", method, path)

	for _, status := range expectedStatuses {
		_, ok := op.Responses[status]
		require.True(t, ok, "operation %s %s missing response status %s", method, path, status)
	}
}

func getOperation(item *openapi3.PathItem, method string) *openapi3.Operation {
	switch method {
	case http.MethodGet:
		return item.Get
	case http.MethodPost:
		return item.Post
	case http.MethodPatch:
		return item.Patch
	case http.MethodDelete:
		return item.Delete
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
