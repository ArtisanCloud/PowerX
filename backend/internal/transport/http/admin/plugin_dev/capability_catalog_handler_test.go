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

	items := buildSyncCapabilities(req.Catalog, exposurePermissions, nil)
	require.Len(t, items, 1)
	require.Equal(t, []string{"com.powerx.plugins.base.local.template:create"}, items[0]["tool_scope"])
	require.Equal(t, []string{"com.powerx.plugins.base.local.template:create"}, items[0]["annotations"].(map[string]interface{})["permission_codes"])
}

func TestBuildSyncCapabilitiesDerivesPermissionCodesFromDescriptorAsset(t *testing.T) {
	req := capabilityCatalogRequest{
		Catalog: capabilityCatalogSnapshot{
			PluginID:        "com.powerx.plugins.base.local",
			ManifestVersion: "1.0.0",
			Entries: []capabilityCatalogEntry{
				{
					ID:         "com.powerx.plugins.base.local.template.create",
					Version:    "1.0.0",
					Descriptor: "contracts/capabilities/template.create.yaml",
				},
			},
		},
		Assets: []capabilityCatalogAsset{
			{
				Type: "descriptor",
				Path: "contracts/capabilities/template.create.yaml",
				Content: base64.StdEncoding.EncodeToString([]byte(`
title_i18n:
  zh-CN: 创建模板
  en: Create template
description_i18n:
  zh-CN: 创建可复用模板。
  en: Create reusable templates.
rbac:
  resource: template
  actions:
    - create
agent:
  risk_level: low
`)),
			},
		},
	}

	descriptors, err := descriptorMetadataFromCatalogAssets(req.Catalog.PluginID, req.Catalog.Entries, req.Assets)
	require.NoError(t, err)

	items := buildSyncCapabilities(req.Catalog, nil, descriptors)
	require.Len(t, items, 1)
	require.Equal(t, "创建模板", items[0]["title"])
	require.Equal(t, "创建可复用模板。", items[0]["description"])
	require.Equal(t, []string{"com.powerx.plugins.base.local.template:create"}, items[0]["tool_scope"])

	annotations := items[0]["annotations"].(map[string]interface{})
	require.Equal(t, []string{"com.powerx.plugins.base.local.template:create"}, annotations["permission_codes"])
	require.Equal(t, "low", annotations["risk_level"])
	require.Equal(t, map[string]string{"zh-CN": "创建模板", "en": "Create template"}, annotations["title_i18n"])
}

func TestBuildSyncCapabilitiesCarriesFineGrainedPluginPermissions(t *testing.T) {
	req := capabilityCatalogRequest{
		Catalog: capabilityCatalogSnapshot{
			PluginID:        "com.powerx.plugins.production.local",
			ManifestVersion: "1.0.0",
			Entries: []capabilityCatalogEntry{
				{
					ID:         "com.powerx.plugins.production.local.sample_track",
					Version:    "1.0.0",
					Descriptor: "contracts/capabilities/sample_track.yaml",
					Protocols: map[string]interface{}{
						"rest": map[string]interface{}{
							"method":         "POST",
							"path":           "/sample-tracks/{uuid}/nodes/sample-schedule",
							"actor_context":  "admin_user",
							"resource_scope": "tenant",
						},
					},
				},
			},
		},
		Assets: []capabilityCatalogAsset{
			{
				Type: "descriptor",
				Path: "contracts/capabilities/sample_track.yaml",
				Content: base64.StdEncoding.EncodeToString([]byte(`
title_i18n:
  zh-CN: 小样跟踪单
  en: Sample tracks
description_i18n:
  zh-CN: 小样跟踪单业务能力。
  en: Sample track capability.
rbac:
  resource: sample_track
  actions:
    - read
permissions:
  - type: action
    permission_code: production.sample_track:factory_schedule
    title_i18n:
      zh-CN: 小样打样排产
      en: Factory schedule
    description_i18n:
      zh-CN: 允许执行小样打样排产。
      en: Allows factory schedule.
    risk_level: medium
    data_scope: tenant
    default_role_grants:
      - role_admin
  - type: api
    permission_code: production.sample_track_api:sample_schedule
    business_permission_code: production.sample_track:factory_schedule
    title_i18n:
      zh-CN: 小样排产接口
      en: Sample schedule API
    description_i18n:
      zh-CN: 允许调用小样排产接口。
      en: Allows sample schedule API calls.
    risk_level: medium
    protocol_bindings:
      - channel: rest
        method: post
        path: /sample-tracks/{uuid}/nodes/sample-schedule
        actor_context: admin_user
        resource_scope: tenant
`)),
			},
		},
	}

	descriptors, err := descriptorMetadataFromCatalogAssets(req.Catalog.PluginID, req.Catalog.Entries, req.Assets)
	require.NoError(t, err)

	items := buildSyncCapabilities(req.Catalog, nil, descriptors)
	require.Len(t, items, 1)
	permissions := items[0]["permissions"].([]catalogPluginPermissionDeclaration)
	require.Len(t, permissions, 2)
	require.Equal(t, "production.sample_track:factory_schedule", permissions[0].PermissionCode)
	require.Equal(t, "POST", permissions[1].ProtocolBindings[0].Method)

	annotations := items[0]["annotations"].(map[string]interface{})
	require.Equal(t, permissions, annotations["permissions"])

	snapshot, err := localPermissionSnapshotFromCatalog(req)
	require.NoError(t, err)
	require.Equal(t, "com.powerx.plugins.production.local", snapshot.PluginID)
	require.Equal(t, "local_mock", snapshot.Source)
	require.Equal(t, []string{
		"production.sample_track:factory_schedule",
		"production.sample_track_api:sample_schedule",
	}, snapshot.PermissionCodes)
	require.NotEmpty(t, snapshot.PermsHash)
	require.Equal(t, "iam:"+snapshot.PermsHash, snapshot.PolicyVersion)
}

func TestDescriptorMetadataFromCatalogAssetsRequiresReferencedDescriptor(t *testing.T) {
	_, err := descriptorMetadataFromCatalogAssets("com.powerx.plugins.base.local", []capabilityCatalogEntry{
		{
			ID:         "com.powerx.plugins.base.local.template.create",
			Descriptor: "contracts/capabilities/template.create.yaml",
		},
	}, nil)
	require.ErrorContains(t, err, "capability descriptor asset missing")
}
