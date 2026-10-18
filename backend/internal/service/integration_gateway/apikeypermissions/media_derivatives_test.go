package apikeypermissions

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaDerivativePermissionPlanes(t *testing.T) {
	dir, err := filepath.Abs("../../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv(platformCapabilitiesDirEnv, dir)
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	for _, action := range []string{"read", "ensure"} {
		service, admin, events := false, false, false
		for _, row := range rows {
			var meta map[string]any
			require.NoError(t, json.Unmarshal(row.Meta, &meta))
			if meta["capability_id"] != "com.corex.media.image_derivatives."+action {
				continue
			}
			if meta["type"] == "capability_service" {
				service = true
				require.True(t, row.AllowAPIKey)
				require.Equal(t, true, meta["api_key_explicit"])
				require.Equal(t, "core://media/image-derivatives", meta["core_internal_endpoint"])
				grant, ok := ResolvePermission(row)
				require.True(t, ok)
				require.Equal(t, "_scope.media.image_derivatives."+action, grant.Scope)
				require.Equal(t, action, grant.Action)
			} else if endpoint, ok := meta["api_endpoint"].(string); ok && strings.Contains(endpoint, "/admin/media/") {
				admin = true
				require.NotEqual(t, true, meta["api_key_explicit"])
			} else if endpoint, ok := meta["api_endpoint"].(string); ok && endpoint == "/api/v1/tenant/media/assets/{asset_uuid}/derivatives/events" {
				events = true
				require.True(t, row.AllowAPIKey)
				require.Equal(t, true, meta["api_key_explicit"])
				grant, ok := ResolvePermission(row)
				require.True(t, ok)
				require.Equal(t, "_scope.media.image_derivatives.read", grant.Scope)
				require.Equal(t, "capability", grant.ResourceType)
			}
		}
		require.True(t, service)
		require.True(t, admin)
		if action == "read" {
			require.True(t, events, "service events share the explicit read grant")
		}
	}
}
