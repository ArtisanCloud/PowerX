package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestLunaVLMRequestAndNestedUsage(t *testing.T) {
	for _, source := range []string{"https://example.invalid/reference.png", "data:image/png;base64,iVBORw0KGgo="} {
		t.Run(source[:4], func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/chat/completions", r.URL.Path)
				require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "gpt-6-luna", body["model"])
				require.Equal(t, "none", body["reasoning_effort"])
				require.EqualValues(t, 2048, body["max_completion_tokens"])
				require.NotContains(t, body, "max_tokens")
				require.EqualValues(t, 0, body["temperature"])
				require.NotContains(t, body, "api_path")
				messages := body["messages"].([]any)
				parts := messages[0].(map[string]any)["content"].([]any)
				require.Equal(t, source, parts[1].(map[string]any)["image_url"].(map[string]any)["url"])
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":[{"type":"text","text":"A test image."}]}}],"usage":{"total_tokens":32,"prompt_tokens":20,"completion_tokens":12,"prompt_tokens_details":{"image_tokens":10},"completion_tokens_details":{"reasoning_tokens":0}}}`))
			}))
			defer srv.Close()
			out, err := NewVLMClient().Invoke(context.Background(), contract.VLMRequest{Messages: []contract.Message{{Role: "user", Content: []contract.ContentPart{{Type: "text", Text: "Describe"}, {Type: "image_url", URL: source}}}}, MaxTokens: 2048, Runtime: map[string]any{"config": &config.ModelConfig{Provider: "openai", Model: "gpt-6-luna", Endpoint: srv.URL + "/v1", APIKey: "fixture-key", Extra: map[string]any{"api_path": "/v1/chat/completions"}}, "temperature_set": true, "reasoning_effort": "none"}})
			require.NoError(t, err)
			require.Equal(t, "A test image.", out.Text)
			require.Equal(t, 32, out.Usage["total_tokens"])
		})
	}
}

func TestLunaVLMReasoningParametersAndProviderErrors(t *testing.T) {
	client := NewVLMClient()
	mc := &config.ModelConfig{Model: "gpt-6-luna"}
	body, err := client.makeBody(mc, contract.VLMRequest{MaxTokens: 900, Runtime: map[string]any{"reasoning_effort": "low"}}, false)
	require.NoError(t, err)
	require.NotContains(t, string(body), "temperature")
	require.NotContains(t, string(body), "top_p")
	_, err = client.makeBody(mc, contract.VLMRequest{Temperature: .5, Runtime: map[string]any{"reasoning_effort": "low"}}, false)
	require.Error(t, err)
	require.Error(t, ValidateVLMParameters("gpt-6-luna", map[string]any{"temperature": 0, "reasoning_effort": "medium"}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "provider-trace")
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"code":"model_not_allowed","message":"private fixture-key"}}`))
	}))
	defer srv.Close()
	_, err = client.Invoke(context.Background(), contract.VLMRequest{Runtime: map[string]any{"config": &config.ModelConfig{Model: "gpt-6-luna", APIKey: "fixture-key", Endpoint: srv.URL}}})
	require.Equal(t, "AI_PROVIDER_REJECTED", dto.CodeOf(err))
	require.Equal(t, "provider-trace", dto.DetailsOf(err)["provider_trace_id"])
	require.NotContains(t, err.Error(), "fixture-key")
}
