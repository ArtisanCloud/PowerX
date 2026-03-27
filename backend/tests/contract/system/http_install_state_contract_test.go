package systemcontract

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestInstallStateHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "025-powerx-docker-systemd", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err)

	assertOperation(t, doc, "/admin/setup/status", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/admin/deploy/releases", http.MethodGet, []string{"200", "503"})

	schema := doc.Components.Schemas["SetupStatus"]
	require.NotNil(t, schema, "SetupStatus schema missing")
	require.Contains(t, schema.Value.Properties, "install_status", "SetupStatus.install_status missing")
	require.Contains(t, schema.Value.Properties, "guard_mode", "SetupStatus.guard_mode missing")
}
