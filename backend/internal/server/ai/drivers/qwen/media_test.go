package qwen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestNativeMediaPreservesNonJSONUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "provider-limit-trace")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("private supplier response"))
	}))
	defer server.Close()
	_, err := nativeMediaCall(context.Background(), &config.ModelConfig{Endpoint: server.URL, APIKey: "test-only"}, map[string]any{})
	var business *dto.AppError
	require.ErrorAs(t, err, &business)
	require.Equal(t, 429, business.HTTPCode)
	require.Equal(t, 429, business.Details["upstream_status"])
	require.Equal(t, "provider-limit-trace", business.Details["provider_trace_id"])
	require.NotContains(t, business.Error(), "private supplier response")
}
