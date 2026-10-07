package apikeypermissions

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/stretchr/testify/require"
)

func TestEnsureTemplatePermissionsRejectsInvalidCatalog(t *testing.T) {
	t.Setenv(platformCapabilitiesDirEnv, filepath.Join(t.TempDir(), "missing"))
	platformPermissionOnce = sync.Once{}
	platformPermissionRows, platformPermissionErr = nil, nil
	t.Cleanup(func() {
		platformPermissionOnce = sync.Once{}
		platformPermissionRows, platformPermissionErr = nil, nil
	})
	// A failed catalog must return before any database IO, never seed templates
	// alone while reporting success.
	err := EnsureTemplatePermissions(context.Background(), &iamrepo.PermissionRepository{})
	require.ErrorContains(t, err, "platform permission catalog")
}

func TestCheckedInPlatformCatalogExplicitAPIKeyMappings(t *testing.T) {
	dir, err := filepath.Abs("../../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv(platformCapabilitiesDirEnv, dir)
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	var found bool
	explicitCapabilities := map[string]bool{}
	for _, row := range rows {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if meta["api_key_explicit"] == true {
			resolved, ok := ResolvePermission(row)
			require.True(t, ok)
			require.NotEmpty(t, resolved.Scope)
			explicitCapabilities[meta["capability_id"].(string)] = true
		}
		if meta["api_endpoint"] == "/api/v1/tenant/capabilities/catalog" && meta["capability_id"] == "com.corex.capabilities.catalog.read" {
			require.Equal(t, true, meta["api_key_explicit"])
			require.True(t, row.AllowAPIKey)
			found = true
		}
	}
	require.True(t, found)
	for _, resource := range []string{"dictionary", "taxonomy", "tag", "resource_type"} {
		for _, action := range []string{"read", "manage"} {
			require.True(t, explicitCapabilities["com.corex.metadata."+resource+"."+action], resource+"."+action)
		}
	}
	for _, capability := range []string{
		"com.corex.iam.members.read", "com.corex.iam.directory.read", "com.corex.iam.authorization.check",
		"com.corex.knowledge.catalog.read", "com.corex.knowledge.space.create", "com.corex.runtime.identity.read",
		"com.corex.media.assets.read", "com.corex.media.assets.manage", "com.corex.media.assets.transfer", "com.corex.media.assets.variants.manage",
		"com.corex.plugin_release.sessions.read", "com.corex.plugin_release.sessions.manage", "com.corex.plugin_release.imports.read", "com.corex.plugin_release.imports.manage",
	} {
		require.True(t, explicitCapabilities[capability], capability)
	}
}

func TestKnowledgeProvisioningPermissionPlanes(t *testing.T) {
	dir, err := filepath.Abs("../../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv(platformCapabilitiesDirEnv, dir)
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	for _, id := range []string{"com.corex.knowledge.catalog.read", "com.corex.knowledge.space.create"} {
		service, admin := false, false
		for _, row := range rows {
			var meta map[string]any
			require.NoError(t, json.Unmarshal(row.Meta, &meta))
			if meta["capability_id"] != id {
				continue
			}
			if meta["api_endpoint"] == "/api/v1/admin/knowledge-spaces/catalog" || meta["api_endpoint"] == "/api/v1/admin/knowledge-spaces" {
				admin = true
				require.NotEqual(t, true, meta["api_key_explicit"])
				if resolved, ok := ResolvePermission(row); ok {
					require.NotEqual(t, "_scope.knowledge.catalog.read", resolved.Scope)
					require.NotEqual(t, "_scope.knowledge.space.create", resolved.Scope)
				}
			}
			if meta["type"] == "capability_service" && meta["api_key_explicit"] == true {
				service = true
				require.True(t, row.AllowAPIKey)
				resolved, ok := ResolvePermission(row)
				require.True(t, ok)
				if id == "com.corex.knowledge.catalog.read" {
					require.Equal(t, "_scope.knowledge.catalog.read", resolved.Scope)
					require.Equal(t, "read", resolved.Action)
					require.Equal(t, "catalog", resolved.ResourcePattern)
				} else {
					require.Equal(t, "_scope.knowledge.space.create", resolved.Scope)
					require.Equal(t, "create", resolved.Action)
					require.Equal(t, "space", resolved.ResourcePattern)
				}
			}
		}
		require.True(t, service, id+" service plane")
		require.True(t, admin, id+" admin plane")
	}
}

func TestRuntimeIdentityExplicitServicePermission(t *testing.T) {
	dir, err := filepath.Abs("../../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv(platformCapabilitiesDirEnv, dir)
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	found := false
	for _, row := range rows {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if meta["capability_id"] != "com.corex.runtime.identity.read" || meta["type"] != "capability_service" {
			continue
		}
		require.Equal(t, "core://runtime/identity", meta["core_internal_endpoint"])
		require.Equal(t, true, meta["api_key_explicit"])
		require.True(t, row.AllowAPIKey)
		resolved, ok := ResolvePermission(row)
		require.True(t, ok)
		require.Equal(t, "_scope.runtime.identity.read", resolved.Scope)
		require.Equal(t, "read", resolved.Action)
		require.Equal(t, "capability", resolved.ResourceType)
		require.Equal(t, "runtime_identity_read", resolved.ResourcePattern)
		found = true
	}
	require.True(t, found)
}
