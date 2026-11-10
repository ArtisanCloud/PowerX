package apikeypermissions

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestLLMStreamFormalContractGeneratesIndependentServiceGrant(t *testing.T) {
	t.Setenv(platformCapabilitiesDirEnv, filepath.Join("..", "..", "..", "..", "config", "platform_capabilities"))
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	found := false
	for _, row := range rows {
		var meta map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &meta))
		if meta["capability_id"] != "com.corex.ai.llm.stream" || meta["type"] != "capability_service" {
			continue
		}
		found = true
		require.True(t, row.AllowAPIKey)
		require.Equal(t, true, meta["api_key_explicit"])
		require.Equal(t, "core://ai/llm/stream", meta["core_internal_endpoint"])
		spec := meta["api_key"].(map[string]any)
		require.Equal(t, "_scope.ai.llm.stream", spec["scope"])
		require.Equal(t, "stream", spec["action"])
		require.Equal(t, "capability", spec["resource_type"])
		require.Equal(t, "ai_llm_stream", spec["resource_pattern"])
	}
	require.True(t, found, "LLM_STREAM_SERVICE_DECLARATION_MISSING")
}
