package modelparams

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestModelParameterSourcesAndIsolation(t *testing.T) {
	p := Known("openai", "gpt-6-luna")
	require.Equal(t, "official_docs", p.Source)
	require.Equal(t, "medium", p.Defaults["reasoning_effort"])
	require.False(t, p.SupportsSampling(""))
	require.True(t, p.SupportsSampling("none"))
	require.NotContains(t, p.Defaults, "max_tokens", "输出上限不能冒充请求默认预算")
	p.Defaults["reasoning_effort"] = "none"
	require.Equal(t, "medium", Known("openai", "gpt-6-luna").Defaults["reasoning_effort"])
	require.Equal(t, "unknown", Known("openrouter", "openai/gpt-6-luna").Source)
	require.Equal(t, "unknown", Known("openai", "unconfirmed-native-id").Source)
	require.False(t, Known("openai", "gpt-6-astra").SupportsSampling("none"))
}

func TestResolveCompatibleGatewayProfiles(t *testing.T) {
	const provider, model = "openai_compatible", "openai/gpt-6-luna"
	stale := &Profile{Provider: provider, Model: model, Source: "unknown", Sampling: "unknown"}
	require.False(t, Resolve(provider, model, stale).SupportsSampling(""))
	require.Equal(t, "unknown", stale.Sampling)

	reported := &Profile{Provider: provider, Model: model, Source: "provider_api", Sampling: "unknown",
		Defaults: map[string]any{"reasoning_effort": "none"}, SupportedParametersKnown: true,
		SupportedParameters: []string{"top_p", "max_tokens"}}
	resolved := Resolve(provider, model, reported)
	require.True(t, resolved.SupportsSampling(""))
	require.False(t, resolved.SupportsSampling("high"))
	require.False(t, resolved.Supports("temperature"))
	require.Equal(t, "max_tokens", resolved.TokenParameter)
	require.Equal(t, model, resolved.Model)
	require.Equal(t, provider, resolved.Provider)
	require.Empty(t, Known(provider, model).Defaults)
}

func TestProviderParametersKeepZeroAndDiscardUndeclaredFields(t *testing.T) {
	p := Provider("openrouter", "custom", "https://provider.invalid/models", "2026-11-02", map[string]any{"temperature": float64(0), "top_p": .8, "api_key": "secret-fixture", "reasoning_effort": "secret-fixture", "max_tokens": float64(512)}, []string{"max_tokens", "temperature", "api_key"}, 32000)
	require.Equal(t, "provider_api", p.Source)
	require.EqualValues(t, 0, p.Defaults["temperature"])
	require.EqualValues(t, 512, p.Defaults["max_tokens"])
	require.Equal(t, 32000, p.MaxOutputTokens)
	require.False(t, p.Supports("top_p"))
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret-fixture")
	require.NotContains(t, string(raw), "api_key")
	require.Equal(t, "official_docs", Provider("openai", "gpt-6-luna", "proxy", "now", map[string]any{"temperature": .7}, nil, 100).Source)
}
