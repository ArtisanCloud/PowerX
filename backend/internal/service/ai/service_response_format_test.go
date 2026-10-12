package ai

import (
	aiconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExplicitJSONObjectModePreservesContractAndRejectsConflicts(t *testing.T) {
	for _, provider := range []string{"openai", "qwen-cn", "ollama"} {
		mc := &aiconfig.ModelConfig{Provider: provider}
		require.NoError(t, applyResponseFormat(mc, map[string]any{"response_format": map[string]any{"type": "json_object"}}))
		require.Equal(t, "json_object", mc.ResponseFormat)
	}
	for _, raw := range []any{nil, "json", map[string]any{}, map[string]any{"type": "text"}, map[string]any{"type": "json_object", "extra": true}} {
		require.ErrorIs(t, applyResponseFormat(&aiconfig.ModelConfig{Provider: "openai"}, map[string]any{"response_format": raw}), ErrInvalidLLMParams)
	}
	conflict := &aiconfig.ModelConfig{Provider: "openai", ResponseSchema: map[string]any{"type": "object"}}
	require.ErrorIs(t, applyResponseFormat(conflict, map[string]any{"response_format": map[string]any{"type": "json_object"}}), ErrInvalidLLMParams)
	require.Empty(t, conflict.ResponseFormat)
	require.ErrorIs(t, applyResponseFormat(&aiconfig.ModelConfig{Provider: "openai"}, map[string]any{"response_format": map[string]any{"type": "json_object"}, "response_schema": "invalid"}), ErrInvalidLLMParams)
	unsupported := &aiconfig.ModelConfig{Provider: "fixture-unsupported"}
	require.ErrorIs(t, applyResponseFormat(unsupported, map[string]any{"response_format": map[string]any{"type": "json_object"}}), ErrInvalidLLMParams)
	require.Empty(t, unsupported.ResponseFormat)
}
