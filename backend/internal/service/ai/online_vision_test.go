package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestFreshOnlineVisionModelUsesTypedDriverAndReturnsAnalysis(t *testing.T) {
	f := newMediaFixture(t)
	discovery, inference := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			discovery++
			_, _ = w.Write([]byte(`{"data":[{"id":"fresh-vision-native"}]}`))
		case "/v1/chat/completions":
			inference++
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "fresh-vision-native", body["model"])
			require.EqualValues(t, 2048, body["max_completion_tokens"])
			require.NotContains(t, body, "temperature")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"A blue reference image."}}]}`))
		default:
			t.Errorf("unexpected provider route %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", ptr(mediaTestTenant), &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "fixture-only", "base_url": srv.URL + "/v1"}}))
	items, err := f.s.ai.ListMediaModels(f.ctx, "dev", mediaTestTenant, "openai", "", catalog.VisionUnderstanding)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "unspecified", items[0].UsageSource)
	require.False(t, items[0].InferenceVerified)
	require.False(t, items[0].Available, "online discovery is not an explicit usage selection or a successful model call")
	require.NoError(t, f.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: ptr(mediaTestTenant), Provider: "openai", Modality: "vlm", Model: "fresh-vision-native", Tags: []string{catalog.OnlineUsageSelectedTag}, Defaults: datatypes.JSONMap{"max_tokens": 2048}}).Error)
	out, err := f.s.ai.VLMInvoke(f.ctx, "dev", mediaTestTenant, "openai/fresh-vision-native", []ContentItem{{Type: "text", Content: "Describe"}, {Type: "image_url", URL: "https://example.invalid/blue.png"}}, nil)
	require.NoError(t, err)
	require.Equal(t, "analysis", out["type"])
	require.NotEmpty(t, out["text"])
	require.Equal(t, 1, discovery)
	require.Equal(t, 1, inference)
}
