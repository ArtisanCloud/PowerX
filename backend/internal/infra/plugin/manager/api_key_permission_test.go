package manager

import (
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestManifestRejectsImplicitAndAdminAPIKeyPermissions(t *testing.T) {
	spec := plugin_mgr.PermissionSpec{Type: "api", PermissionCode: "commerce.plans:read", Independent: true, ProtocolBindings: []plugin_mgr.PermissionProtocolBinding{{ActorContext: "service_actor", ResourceScope: "tenant"}}, APIKey: &plugin_mgr.APIKeyPermissionSpec{CapabilityID: "com.example.plans.read", PluginID: "com.example.plans", Scope: "_scope.plans.read", Action: "read", ResourceType: "capability", ResourcePattern: "plans", Effect: "allow"}}
	require.NoError(t, validateManifestAPIKeyPermission("com.example.plans", spec))
	spec.ProtocolBindings[0].ActorContext = "admin_user"
	require.Error(t, validateManifestAPIKeyPermission("com.example.plans", spec))
	spec.APIKey = nil
	spec.AllowAPIKey = true
	require.Error(t, validateManifestAPIKeyPermission("com.example.plans", spec))
	spec.AllowAPIKey = false
	require.NoError(t, validateManifestAPIKeyPermission("com.example.plans", spec))
}
