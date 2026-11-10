package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/stretchr/testify/require"
)

func TestImage2GenerationUsesImageRouteAndDecodesNestedUsage(t *testing.T) {
	for _, model := range []string{"gpt-image-2", "gpt-image-2-2026-04-21", "chatgpt-image-latest"} {
		t.Run(model, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "/v1/images/generations", r.URL.Path)
				require.Equal(t, "POST", r.Method)
				require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
				var payload map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				require.Equal(t, model, payload["model"])
				require.Equal(t, "medium", payload["quality"])
				require.Equal(t, "png", payload["output_format"])
				require.Equal(t, "1024x1024", payload["size"])
				require.NotContains(t, payload, "response_format")
				_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2UtZml4dHVyZQ=="}],"usage":{"input_tokens":5,"output_tokens":10,"total_tokens":15,"input_tokens_details":{"text_tokens":5,"image_tokens":0}}}`))
			}))
			defer srv.Close()
			out, err := NewImageClient("openai").Generate(context.Background(), contract.ImageRequest{Prompt: "fixture", Size: "1024x1024", Quality: "standard", Format: "png", Runtime: map[string]any{"config": &config.ModelConfig{Endpoint: srv.URL + "/v1", APIKey: "fixture-key", Model: model}}})
			require.NoError(t, err)
			require.Equal(t, [][]byte{[]byte("image-fixture")}, out.Images)
			require.Equal(t, 15, out.Usage["total_tokens"])
			require.Equal(t, 1, calls)
		})
	}
}
