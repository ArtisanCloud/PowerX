package capability_registry

import (
	"context"
	"encoding/json"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func TestToolProtocolPreservesExplicitInstalledSchemaRef(t *testing.T) {
	protocols, err := buildProtocolBindings(map[string]any{"http": map[string]any{"method": "POST", "path": "/internal/tools/parcel-volumes", "schema_ref": "contracts/tools/parcel-volume.v1.json", "actor_context": "admin_user", "auth_type": "core_internal_token", "resource_scope": "tenant"}}, capabilityManifestProvide{})
	require.NoError(t, err)
	require.Len(t, protocols, 1)
	require.Equal(t, "contracts/tools/parcel-volume.v1.json", protocols[0].SchemaRef)
	require.Equal(t, "/_p/example.plugin/internal/tools/parcel-volumes", normalizePluginProtocolEndpoint("example.plugin", " HTTP ", protocols[0].Endpoint))
}

func TestBaseToolDescriptorImportsThroughActualSyncWorker(t *testing.T) {
	root := os.Getenv("POWERX_T204_BASE_ROOT")
	if root == "" {
		root = "../../../../../Plugins/PowerXPlugin"
		if _, err := os.Stat(root); err != nil {
			t.Skip("set POWERX_T204_BASE_ROOT for real base fixture")
		}
	}
	skeleton := filepath.Join(root, "skeleton")
	descriptor := "contracts/capabilities/com.powerx.plugins.base.parcel.volume.yaml"
	provide := capabilityManifestProvide{ID: "com.powerx.plugins.base.parcel.volume", Version: "1.0.0", Descriptor: descriptor}
	capability, err := loadDescriptorCapability(skeleton, provide)
	require.NoError(t, err)
	require.NoError(t, validatePluginPermissionDeclarations("com.powerx.plugins.base.local", provide.ID, capability.Protocols, capability.Permissions))
	require.Len(t, capability.Permissions, 1)
	require.Equal(t, "action", capability.Permissions[0].Type)
	// 同时解析实际 split manifest，避免只手造目录而漏掉 YAML 或权限声明问题。
	for _, relative := range []string{"plugin.d/rbac.yaml", "plugin.d/exposure.yaml", "plugin.d/capabilities.yaml"} {
		raw, err := os.ReadFile(filepath.Join(skeleton, relative))
		require.NoError(t, err)
		var parsed map[string]any
		require.NoError(t, yaml.Unmarshal(raw, &parsed))
	}
	installed := t.TempDir()
	for _, relative := range []string{descriptor, "contracts/tools/parcel-volume.v1.json"} {
		raw, err := os.ReadFile(filepath.Join(skeleton, relative))
		require.NoError(t, err)
		destination := filepath.Join(installed, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0700))
		require.NoError(t, os.WriteFile(destination, raw, 0600))
	}
	manifest := plugin_mgr.Manifest{ID: "com.powerx.plugins.base.local", Version: "1.0.0"}
	raw, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(installed, "plugin.yaml"), raw, 0600))
	bundle := map[string]any{"capabilities": map[string]any{"provides": []any{map[string]any{"id": provide.ID, "version": provide.Version, "descriptor": descriptor, "schemas": map[string]any{"input": "contracts/tools/parcel-volume.v1.json", "output": "contracts/tools/parcel-volume.v1.json"}}}}}
	raw, err = yaml.Marshal(bundle)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(installed, "plugin.d"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(installed, "plugin.d/capabilities.yaml"), raw, 0600))
	db := newMemoryDB(t)
	worker := NewSyncWorker(SyncWorkerConfig{DB: db})
	require.NoError(t, worker.ProcessArtifact(context.Background(), installed))
	record, err := worker.recordRepo.GetByCapabilityID(context.Background(), provide.ID)
	require.NoError(t, err)
	var protocols []models.ProtocolBinding
	require.NoError(t, json.Unmarshal(record.Protocols, &protocols))
	require.Len(t, protocols, 1)
	require.Equal(t, "contracts/tools/parcel-volume.v1.json", protocols[0].SchemaRef)
}
