package capability_registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractStringFromPayload_ReadsTopLevelFirst(t *testing.T) {
	t.Parallel()

	payload := map[string]interface{}{
		"model_key": "top/provider:model",
		"body": map[string]interface{}{
			"model_key": "body/provider:model",
		},
	}

	require.Equal(t, "top/provider:model", extractStringFromPayload(payload, "model_key"))
}

func TestExtractStringFromPayload_ReadsBodyWhenTopLevelMissing(t *testing.T) {
	t.Parallel()

	payload := map[string]interface{}{
		"body": map[string]interface{}{
			"model_key": "ollama/qwen3:8b",
			"modality":  "llm",
		},
	}

	require.Equal(t, "ollama/qwen3:8b", extractStringFromPayload(payload, "model_key"))
	require.Equal(t, "llm", extractStringFromPayload(payload, "modality"))
}

