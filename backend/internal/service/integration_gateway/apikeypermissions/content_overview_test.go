package apikeypermissions

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentOverviewExplicitPermissionPlanes(t *testing.T) {
	dir, err := filepath.Abs("../../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv(platformCapabilitiesDirEnv, dir)
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	service, admin := false, false
	for _, row := range rows {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if meta["capability_id"] != "com.corex.knowledge.content_overview.service_read" {
			continue
		}
		if meta["type"] == "capability_service" {
			service = true
			require.Equal(t, "core://knowledge/content-overview", meta["core_internal_endpoint"])
			require.Equal(t, true, meta["api_key_explicit"])
			require.True(t, row.AllowAPIKey)
			grant, ok := ResolvePermission(row)
			require.True(t, ok)
			require.Equal(t, "_scope.knowledge.content_overview.service_read", grant.Scope)
			require.Equal(t, "read", grant.Action)
			require.Equal(t, "api", grant.ResourceType)
			require.Equal(t, "content_overview", grant.ResourcePattern)
		}
		if meta["api_endpoint"] == "/api/v1/admin/knowledge-spaces/{spaceId}/content-overview" {
			admin = true
			require.NotEqual(t, true, meta["api_key_explicit"])
			if grant, ok := ResolvePermission(row); ok {
				require.NotEqual(t, "_scope.knowledge.content_overview.service_read", grant.Scope)
			}
		}
	}
	require.True(t, service)
	require.True(t, admin)
}
