package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSCRMServiceDescriptorsPassInstallationManifestPermissionMapping(t *testing.T) {
	const fixture = "../service/capability_registry/testdata/scrm-service-registration"
	const directory = "specs/008-channel-provider-messaging/contracts/service-descriptors"
	for _, base := range []string{"/api/v1", "/custom/api/v7"} {
		t.Run(strings.ReplaceAll(base, "/", "_"), func(t *testing.T) {
			root := t.TempDir()
			permissions := []any{}
			files, err := filepath.Glob(filepath.Join(fixture, directory, "*.yaml"))
			require.NoError(t, err)
			require.Len(t, files, 5)
			for _, file := range files {
				raw, err := os.ReadFile(file)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(raw, &doc))
				for _, p := range doc["permissions"].([]any) {
					for _, b := range p.(map[string]any)["protocol_bindings"].([]any) {
						binding := b.(map[string]any)
						binding["path"] = base + strings.TrimPrefix(binding["path"].(string), "/api/v1")
					}
					permissions = append(permissions, p)
				}
			}
			raw, err := yaml.Marshal(map[string]any{"id": "com.powerx.plugins.scrm", "version": "1.0.0", "runtime": map[string]any{"kind": "process", "entry": "fixture"}, "endpoints": map[string]any{"http_base_path": base}, "permissions": permissions})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), raw, 0600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "fixture"), []byte("never executed"), 0600))
			descriptor, err := pmimpl.NewFSLoader().LoadDescriptor(context.Background(), root)
			require.NoError(t, err)
			db := newPluginAuthzTestDB(t)
			for repeat := 0; repeat < 2; repeat++ {
				require.NoError(t, syncPluginManifestPermissions(context.Background(), db, descriptor.Manifest))
			}
			var stored []iam.Permission
			require.NoError(t, db.Where("source=?", "plugin:com.powerx.plugins.scrm").Find(&stored).Error)
			require.Len(t, stored, 5)
			for _, row := range stored {
				require.True(t, row.AllowAPIKey)
			}
			rows := buildPluginPermissionRows(descriptor.Manifest)
			require.Len(t, rows, 5)
			for _, row := range rows {
				require.True(t, row.AllowAPIKey)
				var meta map[string]any
				require.NoError(t, json.Unmarshal(row.Meta, &meta))
				require.Equal(t, true, meta["api_key_explicit"])
				require.Equal(t, true, meta["independent"])
				require.Equal(t, "com.powerx.plugins.scrm", meta["api_key"].(map[string]any)["plugin_id"])
				binding := meta["protocol_bindings"].([]any)[0].(map[string]any)
				require.True(t, strings.HasPrefix(binding["path"].(string), base+"/service/"))
				require.Equal(t, "service_actor", binding["actor_context"])
				require.Equal(t, "tenant", binding["resource_scope"])
			}
		})
	}
}
