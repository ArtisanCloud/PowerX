package opscontract

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestMigrationHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "025-powerx-docker-systemd", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load migration openapi spec")

	assertOperation(t, doc, "/admin/migration/runbooks/run", http.MethodPost, []string{"200"})
	assertOperation(t, doc, "/admin/migration/runbooks/{migrationId}", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/admin/migration/runbooks/{migrationId}/acceptance", http.MethodPost, []string{"200"})
	assertOperation(t, doc, "/admin/migration/traffic/switch", http.MethodPost, []string{"200"})

	schema := doc.Components.Schemas["MigrationRunbookRecord"]
	require.NotNil(t, schema, "MigrationRunbookRecord schema missing")
}
