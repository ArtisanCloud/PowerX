//go:build ignore

package mediacontract

import (
	"net/http"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// TestMediaOpenAPIContract ensures the public Media OpenAPI keeps the published
// endpoints for宿主/插件使用的能力接口。
func TestMediaOpenAPIContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "001-media-storage", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load media openapi spec")

	require.NotNil(t, doc.Paths)
	assertOperation(t, doc, "/media/assets", http.MethodPost, []string{"201", "400"})
	assertOperation(t, doc, "/media/assets", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/media/assets/{uuid}", http.MethodGet, []string{"200", "404"})
	assertOperation(t, doc, "/media/assets/{uuid}", http.MethodDelete, []string{"200", "404"})
	assertOperation(t, doc, "/media/assets/{uuid}/presign", http.MethodPost, []string{"200", "404"})

	schema := doc.Components.Schemas["CapabilityRecordDTO"]
	require.NotNil(t, schema, "CapabilityRecordDTO schema missing")
}

func assertOperation(t testing.TB, doc *openapi3.T, path, method string, statuses []string) {
	t.Helper()
	item, ok := doc.Paths[path]
	require.True(t, ok, "path %s missing", path)
	op := getOperation(item, method)
	require.NotNil(t, op, "operation %s %s missing", method, path)
	for _, status := range statuses {
		_, ok := op.Responses[status]
		require.True(t, ok, "status %s missing on %s %s", status, method, path)
	}
}

func getOperation(item *openapi3.PathItem, method string) *openapi3.Operation {
	switch method {
	case http.MethodGet:
		return item.Get
	case http.MethodPost:
		return item.Post
	case http.MethodDelete:
		return item.Delete
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
