package skills

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePluginRegistryManifestRequiresPrepareCapability(t *testing.T) {
	manifest := map[string]any{
		"executor": map[string]any{
			"type":       "capability",
			"capability": "powerxplugin.template",
			"action_map": map[string]any{
				"create": "com.powerx.plugins.base.local.template.create",
			},
		},
	}

	err := validatePluginRegistryManifest(manifest)
	require.ErrorContains(t, err, "executor.prepare_capability is required")
}

func TestValidatePluginRegistryManifestAcceptsPrepareCapability(t *testing.T) {
	manifest := map[string]any{
		"executor": map[string]any{
			"type":               "capability",
			"capability":         "powerxplugin.template",
			"prepare_capability": "com.powerx.plugins.base.local.template.prepare",
			"action_map": map[string]any{
				"create": "com.powerx.plugins.base.local.template.create",
			},
		},
	}

	require.NoError(t, validatePluginRegistryManifest(manifest))
}
