package pluginpermission

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPluginServiceAPIKeyRequiresExplicitOwnedBinding(t *testing.T) {
	const plugin = "com.powerx.plugins.test"
	spec := APIKeySpec{CapabilityID: plugin + ".plans.read", Scope: "_scope.test.plans.read", Action: "read", ResourceType: "capability", ResourcePattern: "plans", PluginID: plugin, Effect: "allow"}
	meta, err := ExplicitAPIKeyMetadata(&spec, plugin, spec.CapabilityID, true)
	require.NoError(t, err)
	require.Equal(t, true, meta["api_key_explicit"])
	for _, mutate := range []func(*APIKeySpec){
		func(s *APIKeySpec) { s.PluginID = "other.plugin" },
		func(s *APIKeySpec) { s.CapabilityID = "other.plugin.plans.read" },
		func(s *APIKeySpec) { s.Scope = "*" },
		func(s *APIKeySpec) { s.Effect = "deny" },
	} {
		value := spec
		mutate(&value)
		_, err := ExplicitAPIKeyMetadata(&value, plugin, spec.CapabilityID, true)
		require.Error(t, err)
	}
	_, err = ExplicitAPIKeyMetadata(&spec, plugin, spec.CapabilityID, false)
	require.Error(t, err)
	meta, err = ExplicitAPIKeyMetadata(nil, plugin, spec.CapabilityID, true)
	require.NoError(t, err)
	require.Nil(t, meta)
}
