package apikeypermissions

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnectionProbePermissionSeparatesServiceGrantFromAdminREST(t *testing.T) {
	dir, err := filepath.Abs("../../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv(platformCapabilitiesDirEnv, dir)
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	service, admin := false, false
	for _, row := range rows {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if meta["capability_id"] != "com.corex.ai.connection.test" {
			continue
		}
		switch meta["type"] {
		case "capability_service":
			service = true
			require.True(t, row.AllowAPIKey)
			require.Equal(t, true, meta["api_key_explicit"])
			require.Equal(t, "core://ai/connection", meta["core_internal_endpoint"])
			permission, ok := ResolvePermission(row)
			require.True(t, ok)
			require.Equal(t, ResolvedPermission{Scope: "_scope.ai.connection.test", Action: "test", ResourceType: "api", ResourcePattern: "ai-connection-test", Effect: "allow"}, permission)
		case "api":
			admin = true
			require.False(t, row.AllowAPIKey)
			require.NotEqual(t, true, meta["api_key_explicit"])
		}
	}
	require.True(t, service)
	require.True(t, admin)
}
