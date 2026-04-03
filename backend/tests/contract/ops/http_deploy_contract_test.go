package opscontract

import (
	"net/http"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestDeployOpenAPIContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "025-powerx-docker-systemd", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load deploy openapi spec")

	assertOperation(t, doc, "/admin/deploy/releases", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/admin/deploy/releases", http.MethodPost, []string{"200"})
	assertOperation(t, doc, "/admin/deploy/rollback", http.MethodPost, []string{"200"})
	assertOperation(t, doc, "/admin/deploy/health", http.MethodGet, []string{"200"})

	schema := doc.Components.Schemas["DeployReleaseRecord"]
	require.NotNil(t, schema, "DeployReleaseRecord schema missing")
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
