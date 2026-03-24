package opscontract

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestBackupHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "025-powerx-docker-systemd", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load backup openapi spec")

	assertOperation(t, doc, "/admin/backup/policies", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/admin/backup/policies", http.MethodPost, []string{"200"})
	assertOperation(t, doc, "/admin/backup/jobs/run", http.MethodPost, []string{"200"})
	assertOperation(t, doc, "/admin/backup/jobs", http.MethodGet, []string{"200"})
	assertOperation(t, doc, "/admin/backup/cleanup", http.MethodPost, []string{"200"})
	assertOperation(t, doc, "/admin/backup/restore-drills/run", http.MethodPost, []string{"200"})

	schema := doc.Components.Schemas["BackupJob"]
	require.NotNil(t, schema, "BackupJob schema missing")
}
