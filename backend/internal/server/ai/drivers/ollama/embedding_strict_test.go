package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStrictEmbeddingPreservesCompleteInputAndRejectsLegacyFallback(t *testing.T) {
	text := strings.Repeat("完整需求😀", 500)
	for _, status := range []int{200, 404} {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			require.Equal(t, "/api/embed", r.URL.Path)
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, text, body["input"])
			require.Equal(t, false, body["truncate"])
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"embeddings":[[1,0.5]]}`))
		}))
		driver := OllamaEmbedder{BaseURL: server.URL, Model: "real-model", StrictInput: true}
		vectors, err := driver.Embed(context.Background(), []string{text})
		if status == 200 {
			require.NoError(t, err)
			require.Len(t, vectors, 1)
		} else {
			require.Error(t, err)
			require.Contains(t, err.Error(), "strict embeddings")
		}
		require.Equal(t, 1, requests)
		server.Close()
	}
}
