package capability_registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractStringFromBody_IgnoresTopLevel(t *testing.T) {
	t.Parallel()

	payload := map[string]interface{}{
		"model_key": "top/provider:model",
		"body": map[string]interface{}{
			"model_key": "body/provider:model",
		},
	}

	require.Equal(t, "body/provider:model", extractStringFromBody(payload, "model_key"))
}

func TestExtractStringFromBody_ReadsBodyWhenTopLevelMissing(t *testing.T) {
	t.Parallel()

	payload := map[string]interface{}{
		"body": map[string]interface{}{
			"model_key": "ollama/qwen3:8b",
			"modality":  "llm",
		},
	}

	require.Equal(t, "ollama/qwen3:8b", extractStringFromBody(payload, "model_key"))
	require.Equal(t, "llm", extractStringFromBody(payload, "modality"))
}
