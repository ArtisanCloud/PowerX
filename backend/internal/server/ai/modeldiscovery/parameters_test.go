package modeldiscovery

import (
	"context"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoveryPreservesProviderParameterDefaultsAndLimits(t *testing.T) {
	for _, tc := range []struct {
		provider, response string
		temperature, topP  float64
		output             int
	}{
		{"openrouter", `{"data":[{"id":"vendor/native","default_parameters":{"temperature":0,"top_p":0.8,"api_key":"do-not-store"},"supported_parameters":["temperature","top_p","max_tokens"],"top_provider":{"max_completion_tokens":12345}}]}`, 0, .8, 12345},
		{"google", `{"models":[{"name":"models/gemini-fixture","temperature":1,"topP":0.95,"outputTokenLimit":8192}]}`, 1, .95, 8192},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.response)) }))
			defer srv.Close()
			out, err := Fetch(context.Background(), Input{Provider: tc.provider, Endpoint: srv.URL, APIKey: "fixture-key"})
			require.NoError(t, err)
			require.Len(t, out.Models, 1)
			p := out.Models[0].ParameterProfile
			require.Equal(t, "provider_api", p.Source)
			require.EqualValues(t, tc.temperature, p.Defaults["temperature"])
			require.EqualValues(t, tc.topP, p.Defaults["top_p"])
			require.Equal(t, tc.output, p.MaxOutputTokens)
			require.NotContains(t, p.Defaults, "max_tokens", "输出上限不得当成默认预算")
			require.NotContains(t, p.Defaults, "api_key")
		})
	}
}
