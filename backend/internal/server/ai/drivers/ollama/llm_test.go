package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
	"github.com/stretchr/testify/require"
)

func TestMakeBodyPassesTypedResponseSchemaAsOllamaFormat(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []string{"schema"}}
	body, err := NewLLMClient().makeBody(&config.ModelConfig{
		Model:          "qwen3:8b",
		ResponseSchema: schema,
	}, "test", false)
	require.NoError(t, err)

	var request map[string]any
	require.NoError(t, json.Unmarshal(body, &request))
	format := request["format"].(map[string]any)
	require.Equal(t, "object", format["type"])
	require.Equal(t, []any{"schema"}, format["required"])
}

func TestMakeBodySendsExplicitZeroTemperatureAndSeed(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		body, err := NewLLMClient().makeBody(&config.ModelConfig{
			Model:          "qwen3:8b",
			Temperature:    0,
			TemperatureSet: true,
			Extra:          map[string]any{"seed": int64(42)},
		}, "same prompt", streaming)
		require.NoError(t, err)
		var request map[string]any
		require.NoError(t, json.Unmarshal(body, &request))
		options := request["options"].(map[string]any)
		require.Equal(t, float64(0), options["temperature"])
		require.Equal(t, float64(42), options["seed"])
		require.Equal(t, streaming, request["stream"])
	}
}

func TestMakeBodyOmitsUnspecifiedZeroTemperature(t *testing.T) {
	body, err := NewLLMClient().makeBody(&config.ModelConfig{Model: "qwen3:8b"}, "prompt", false)
	require.NoError(t, err)
	var request map[string]any
	require.NoError(t, json.Unmarshal(body, &request))
	_, hasOptions := request["options"]
	require.False(t, hasOptions)
}

func TestInvokeReturnsProviderTimeoutPhaseWhenOllamaDoesNotSendHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	_, err := NewLLMClient().Invoke(context.Background(), &config.ModelConfig{
		Provider: "ollama",
		Model:    "qwen3:8b",
		Endpoint: server.URL,
		Timeout:  20 * time.Millisecond,
	}, "test")

	require.Error(t, err)
	var providerErr *core.ProviderCallError
	require.True(t, errors.As(err, &providerErr))
	require.True(t, providerErr.IsTimeout())
	require.Equal(t, "response_headers", providerErr.Phase)
	require.Equal(t, "AI_PROVIDER_TIMEOUT", providerErr.Details()["reason_code"])
}
