package skills

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestManifestModelParametersAreExplicitAndBounded(t *testing.T) {
	p, err := ManifestLLMParameters(map[string]any{"parameters": map[string]any{"thinking": false, "max_tokens": 4096}})
	require.NoError(t, err)
	require.Equal(t, false, p["thinking"])
	require.Equal(t, 4096, p["max_tokens"])
	for _, params := range []any{map[string]any{"timeout": 900}, map[string]any{"thinking": "false"}, map[string]any{"max_tokens": 0}, map[string]any{"temperature": 3}, "invalid"} {
		_, err := ManifestLLMParameters(map[string]any{"parameters": params})
		require.Error(t, err)
	}
	p, err = ManifestLLMParameters(nil)
	require.NoError(t, err)
	require.Empty(t, p)
}
