package plugin_dev

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSyncCapabilitiesDerivesPermissionCodesFromExposureAsset(t *testing.T) {
	req := capabilityCatalogRequest{
		Catalog: capabilityCatalogSnapshot{
			PluginID:        "com.powerx.plugins.base.local",
			ManifestVersion: "1.0.0",
			Entries: []capabilityCatalogEntry{
				{
					ID:      "com.powerx.plugins.base.local.template.create",
					Version: "1.0.0",
					Protocols: map[string]interface{}{
						"rest": map[string]interface{}{
							"method":   "POST",
							"endpoint": "/api/v1/templates",
						},
					},
				},
			},
		},
		Assets: []capabilityCatalogAsset{
			{
				Type: "manifest",
				Path: "plugin.d/exposure.yaml",
				Content: base64.StdEncoding.EncodeToString([]byte(`
exposure:
  channels:
    - type: rest
      method: POST
      entrypoint: /api/v1/templates
      capability: com.powerx.plugins.base.local.template.create
      rbac: template:create
`)),
			},
		},
	}

	exposurePermissions, err := permissionCodesFromCatalogAssets(req.Catalog.PluginID, req.Assets)
	require.NoError(t, err)

	items := buildSyncCapabilities(req.Catalog, exposurePermissions)
	require.Len(t, items, 1)
	require.Equal(t, []string{"com.powerx.plugins.base.local.template:create"}, items[0]["tool_scope"])
	require.Equal(t, []string{"com.powerx.plugins.base.local.template:create"}, items[0]["annotations"].(map[string]interface{})["permission_codes"])
}
