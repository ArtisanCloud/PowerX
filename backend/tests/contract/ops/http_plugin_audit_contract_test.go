package opscontract

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestPluginAuditHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "025-powerx-docker-systemd", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load plugin audit openapi spec")

	item, ok := doc.Paths["/admin/plugins/{pluginId}/audit"]
	require.True(t, ok, "path /admin/plugins/{pluginId}/audit missing")
	require.NotNil(t, item.Get, "GET /admin/plugins/{pluginId}/audit missing")
	_, ok = item.Get.Responses["200"]
	require.True(t, ok, "GET /admin/plugins/{pluginId}/audit status 200 missing")

	require.Equal(t, http.MethodGet, "GET")
}
