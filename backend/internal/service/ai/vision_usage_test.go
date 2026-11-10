package ai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestKnownIncompatibleVisionModelIsVisibleButCannotInvoke(t *testing.T) {
	f := newMediaFixture(t)
	generation := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"tts-1"},{"id":"gpt-5.4-mini"},{"id":"future-native"}]}`))
			return
		}
		generation++
		t.Errorf("incompatible vision request reached provider: %s", r.URL.Path)
	}))
	defer srv.Close()
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", ptr(mediaTestTenant), &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "fixture-only", "base_url": srv.URL + "/v1"}}))
	// 旧版保存的错误配置保留，只阻止继续使用；目录查询不得偷偷换型号。
	profile := am.AIModelProfile{Env: "dev", TenantUUID: ptr(mediaTestTenant), Provider: "openai", Modality: "vlm", Model: "tts-1", Tags: []string{catalog.OnlineUsageSelectedTag}, Defaults: datatypes.JSONMap{"max_tokens": 2048}}
	require.NoError(t, f.db.Create(&profile).Error)
	items, err := f.s.ai.ListMediaModels(f.ctx, "dev", mediaTestTenant, "openai", "", catalog.VisionUnderstanding)
	require.NoError(t, err)
	require.Len(t, items, 3)
	for _, item := range items {
		if item.Model == "tts-1" {
			require.False(t, item.Available)
			require.Equal(t, "AI_VISION_IMAGE_INPUT_UNSUPPORTED", item.ReasonCode)
			require.Equal(t, "official_docs", item.Vision.Source)
			require.False(t, item.InferenceVerified)
		}
	}
	_, err = f.s.ai.VLMInvoke(f.ctx, "dev", mediaTestTenant, "openai/tts-1", []ContentItem{{Type: "text", Content: "Describe"}, {Type: "image_url", URL: "https://example.invalid/blue.png"}}, nil)
	require.Equal(t, "AI_VISION_IMAGE_INPUT_UNSUPPORTED", dto.CodeOf(err))
	require.Zero(t, generation)
	var after am.AIModelProfile
	require.NoError(t, f.db.First(&after, profile.ID).Error)
	require.Equal(t, "tts-1", after.Model)
}
