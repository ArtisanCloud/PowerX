package apikeypermissions

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestAIMediaTypedPermissionsKeepAdminAndServicePlanes(t *testing.T) {
	dir, err := filepath.Abs("../../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv(platformCapabilitiesDirEnv, dir)
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	ids := []string{"com.corex.ai.catalog.models.read", "com.corex.ai.catalog.providers.read", "com.corex.ai.vlm.invoke", "com.corex.ai.image.invoke", "com.corex.ai.video.invoke", "com.corex.ai.tts.invoke", "com.corex.ai.media.tasks.read", "com.corex.ai.media.tasks.cancel"}
	for _, id := range ids {
		service := false
		for _, row := range rows {
			var meta map[string]any
			require.NoError(t, json.Unmarshal(row.Meta, &meta))
			if meta["capability_id"] != id {
				continue
			}
			if meta["type"] == "capability_service" {
				service = true
				require.True(t, row.AllowAPIKey)
				require.Equal(t, true, meta["api_key_explicit"])
				grant, ok := ResolvePermission(row)
				require.True(t, ok)
				require.NotEmpty(t, grant.Scope)
			}
			if endpoint, _ := meta["api_endpoint"].(string); endpoint == "/api/v1/admin/agents/models" || endpoint == "/api/v1/admin/agents/providers" {
				require.NotEqual(t, true, meta["api_key_explicit"])
			}
		}
		require.True(t, service, id)
	}
}
