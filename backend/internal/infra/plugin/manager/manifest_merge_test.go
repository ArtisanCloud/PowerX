package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestLoadManifestWithCatalogs_MergeRBAC(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "plugin.yaml", `
id: com.powerx.plugins.base
name: base
version: 0.8.0
runtime:
  kind: process
  entry: backend/bin/plugin
endpoints:
  http_base_path: /api/v1
frontend:
  admin:
    kind: process
catalogs:
  rbac: plugin.d/rbac.yaml
`)
	writeTestFile(t, root, "plugin.d/rbac.yaml", `
rbac:
  resources:
    - resource: base:template
      actions: [read, create, update, delete]
permissions:
  - resource: base:template
    actions: [read]
routes:
  basePath: /api/v1
  rbac: strict
`)

	manifest, err := loadManifestWithCatalogs(root)
	require.NoError(t, err)
	require.Len(t, manifest.RBAC.Resources, 1)
	require.Equal(t, "base:template", manifest.RBAC.Resources[0].Resource)
	require.Len(t, manifest.Permissions, 1)
	require.NotNil(t, manifest.Routes)
	require.Equal(t, "/api/v1", manifest.Routes.BasePath)
}

func TestLoadManifestWithCatalogs_MergeEvents(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "plugin.yaml", `
id: com.powerx.plugins.base
name: base
version: 0.8.0
runtime:
  kind: process
  entry: backend/bin/plugin
endpoints:
  http_base_path: /api/v1
frontend:
  admin:
    kind: process
catalogs:
  events: plugin.d/events.yaml
`)
	writeTestFile(t, root, "plugin.d/events.yaml", `
events:
  publish:
    - _topic.template.updated
  subscribe:
    - _topic.template.created
`)

	manifest, err := loadManifestWithCatalogs(root)
	require.NoError(t, err)
	require.Equal(t, plugin_mgr.EventSpec{
		Publish:   []string{"_topic.template.updated"},
		Subscribe: []string{"_topic.template.created"},
	}, manifest.Events)
}

func TestLoadManifestWithCatalogs_Conflict(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "plugin.yaml", `
id: com.powerx.plugins.base
name: base
version: 0.8.0
runtime:
  kind: process
  entry: backend/bin/plugin
endpoints:
  http_base_path: /api/v1
frontend:
  admin:
    kind: process
rbac:
  resources:
    - resource: base:template
      actions: [read]
catalogs:
  rbac: plugin.d/rbac.yaml
`)
	writeTestFile(t, root, "plugin.d/rbac.yaml", `
rbac:
  resources:
    - resource: base:template
      actions: [read, create]
`)

	_, err := loadManifestWithCatalogs(root)
	require.Error(t, err)
	require.True(t, plugin_mgr.IsCode(err, plugin_mgr.CodeInvalidManifest))
	require.Contains(t, strings.ToLower(err.Error()), "catalog conflict")
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
