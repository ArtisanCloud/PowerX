package plugincontract

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestTenantPluginInstanceHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "026-iam", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load 026-iam openapi spec")

	listPath, ok := doc.Paths["/admin/plugins/tenant-instances"]
	require.True(t, ok, "missing tenant plugin instance list path")
	require.NotNil(t, listPath.Get, "tenant plugin instances must be listed with GET")
	require.Equal(t, "listTenantPluginInstances", listPath.Get.OperationID)
	require.Contains(t, listPath.Get.Tags, "TenantPlugins")
	require.Contains(t, listPath.Get.Responses, "200")

	enablePath, ok := doc.Paths["/admin/plugins/tenant-instances/{plugin_id}/enable"]
	require.True(t, ok, "missing tenant plugin instance enable path")
	require.NotNil(t, enablePath.Post, "tenant plugin instance enable must be POST")
	require.Equal(t, "enableTenantPluginInstance", enablePath.Post.OperationID)
	require.Contains(t, enablePath.Post.Tags, "TenantPlugins")
	require.Contains(t, enablePath.Post.Responses, "200")
	requirePathParam(t, enablePath.Post.Parameters, "plugin_id")

	disablePath, ok := doc.Paths["/admin/plugins/tenant-instances/{plugin_id}/disable"]
	require.True(t, ok, "missing tenant plugin instance disable path")
	require.NotNil(t, disablePath.Post, "tenant plugin instance disable must be POST")
	require.Equal(t, "disableTenantPluginInstance", disablePath.Post.OperationID)
	require.Contains(t, disablePath.Post.Tags, "TenantPlugins")
	require.Contains(t, disablePath.Post.Responses, "200")
	requirePathParam(t, disablePath.Post.Parameters, "plugin_id")

	instance := doc.Components.Schemas["TenantPluginInstance"]
	require.NotNil(t, instance, "TenantPluginInstance schema missing")
	for _, field := range []string{
		"tenant_uuid",
		"plugin_id",
		"version",
		"enabled",
		"config",
	} {
		require.Contains(t, instance.Value.Properties, field, "TenantPluginInstance missing %s", field)
	}
	require.Equal(t, "uuid", instance.Value.Properties["tenant_uuid"].Value.Format)
	require.Equal(t, "boolean", instance.Value.Properties["enabled"].Value.Type)

	listResp := doc.Components.Schemas["TenantPluginInstanceListResponse"]
	require.NotNil(t, listResp, "TenantPluginInstanceListResponse schema missing")
	dataSchema := listResp.Value.Properties["data"].Value
	require.NotNil(t, dataSchema)
	require.Equal(t, "array", dataSchema.Type)
	require.Equal(t, "#/components/schemas/TenantPluginInstance", dataSchema.Items.Ref)
}

func requirePathParam(t *testing.T, params openapi3.Parameters, name string) {
	t.Helper()
	for _, param := range params {
		if param.Value != nil && param.Value.Name == name && param.Value.In == "path" && param.Value.Required {
			return
		}
	}
	t.Fatalf("missing required path parameter %q", name)
}

func repoRootFromHere(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve current file path")
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
