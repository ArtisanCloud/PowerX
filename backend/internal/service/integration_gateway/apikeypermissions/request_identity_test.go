package apikeypermissions

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestRequestIdentityPermissionPlanes(t *testing.T) {
	dir, err := filepath.Abs("../../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv(platformCapabilitiesDirEnv, dir)
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	service, admin := false, false
	for _, row := range rows {
		var meta map[string]interface{}
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if meta["capability_id"] != "com.corex.iam.identity_context.service_read" {
			continue
		}
		if meta["type"] == "capability_service" {
			service = true
			require.True(t, row.AllowAPIKey)
			require.Equal(t, true, meta["api_key_explicit"])
			require.Equal(t, "core://iam/identity-context", meta["core_internal_endpoint"])
			grant, ok := ResolvePermission(row)
			require.True(t, ok)
			require.Equal(t, "_scope.iam.identity_context.service_read", grant.Scope)
			require.Equal(t, "read", grant.Action)
			require.Equal(t, "capability", grant.ResourceType)
			require.Equal(t, "iam_identity_context_read", grant.ResourcePattern)
		}
		if meta["api_endpoint"] == "/api/v1/admin/iam/identity-contexts:resolve" {
			admin = true
			require.NotEqual(t, true, meta["api_key_explicit"])
			if grant, ok := ResolvePermission(row); ok {
				require.NotEqual(t, "_scope.iam.identity_context.service_read", grant.Scope)
			}
		}
	}
	require.True(t, service)
	require.True(t, admin)
}
