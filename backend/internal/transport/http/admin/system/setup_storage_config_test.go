package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyStorageConfig_LocalGeneratesUploadTokenSecret(t *testing.T) {
	root := map[string]any{
		"storage": map[string]any{
			"local": map[string]any{},
		},
	}

	err := applyStorageConfig(root, setupStorageConfig{
		Type:      "local",
		LocalPath: "/opt/powerx/storage/media",
	})
	require.NoError(t, err)

	storage := asMap(root["storage"])
	local := asMap(storage["local"])
	assert.Equal(t, "local", storage["default_driver"])
	assert.Equal(t, "/opt/powerx/storage/media", local["base_path"])
	secret, ok := local["upload_token_secret"].(string)
	require.True(t, ok)
	assert.Len(t, secret, 64)
}

func TestApplyStorageConfig_LocalPreservesExistingUploadTokenSecret(t *testing.T) {
	root := map[string]any{
		"storage": map[string]any{
			"local": map[string]any{
				"upload_token_secret": "existing-secret",
			},
		},
	}

	err := applyStorageConfig(root, setupStorageConfig{
		Type:      "local",
		LocalPath: "/opt/powerx/storage/media",
	})
	require.NoError(t, err)

	local := asMap(asMap(root["storage"])["local"])
	assert.Equal(t, "existing-secret", local["upload_token_secret"])
}
