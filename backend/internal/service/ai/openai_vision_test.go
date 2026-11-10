package ai

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestOpenAIVisionCatalogToAnalysisWithBase64AndNoImageRebinding(t *testing.T) {
	f := newMediaFixture(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-6-luna"}]}`))
			return
		}
		calls++
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "none", body["reasoning_effort"])
		require.Contains(t, string(mustJSON(t, body)), "data:image/png;base64,")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"A small test picture."}}],"usage":{"total_tokens":12,"completion_tokens_details":{"reasoning_tokens":0}}}`))
	}))
	defer srv.Close()
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", ptr(mediaTestTenant), &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "fixture-key", "base_url": srv.URL + "/v1"}}))
	items, err := f.s.ai.ListMediaModels(f.ctx, "dev", mediaTestTenant, "openai", "", catalog.VisionUnderstanding)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.False(t, items[0].Configured)
	profile := am.AIModelProfile{Env: "dev", TenantUUID: ptr(mediaTestTenant), Modality: "vlm", Provider: "openai", Model: "gpt-6-luna", Defaults: datatypes.JSONMap{"max_tokens": 2048, "reasoning_effort": "none"}}
	require.NoError(t, f.db.Create(&profile).Error)
	items, err = f.s.ai.ListMediaModels(f.ctx, "dev", mediaTestTenant, "openai", "", catalog.VisionUnderstanding)
	require.NoError(t, err)
	require.True(t, items[0].Configured)
	require.True(t, items[0].Available)
	require.NotEmpty(t, items[0].Operations[catalog.VisionUnderstanding].ParameterSchema)
	before := []am.AIModelProfile{}
	require.NoError(t, f.db.Where("modality = ?", "image").Find(&before).Error)
	inputs := []ContentItem{{Type: "text", Content: "Describe the picture."}, {Type: "image_base64", Content: base64.StdEncoding.EncodeToString(f.png)}}
	out, err := f.s.ai.VLMInvoke(f.ctx, "dev", mediaTestTenant, "openai/gpt-6-luna", inputs, nil)
	require.NoError(t, err)
	require.Equal(t, "analysis", out["type"])
	require.NotEmpty(t, out["text"])
	require.Equal(t, 1, calls)
	_, err = f.s.ai.VLMInvoke(f.ctx, "dev", mediaTestTenant, "openai/gpt-6-luna", inputs, map[string]any{"reasoning_effort": "low", "temperature": 0})
	require.Equal(t, "AI_PARAMETER_INVALID", dto.CodeOf(err))
	require.Equal(t, 1, calls)
	bad := []ContentItem{{Type: "text", Content: "Describe"}, {Type: "image_base64", Content: "not-base64"}}
	_, err = f.s.ai.VLMInvoke(f.ctx, "dev", mediaTestTenant, "openai/gpt-6-luna", bad, nil)
	require.Equal(t, "AI_INPUT_INVALID", dto.CodeOf(err))
	require.Equal(t, 1, calls)
	after := []am.AIModelProfile{}
	require.NoError(t, f.db.Where("modality = ?", "image").Find(&after).Error)
	require.Equal(t, before, after)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

func TestLegacyVisionProfileWithProviderAliasRemainsConfigured(t *testing.T) {
	f := newMediaFixture(t)
	require.NoError(t, f.db.Model(&am.AIModelProfile{}).Where("modality = ? AND model = ?", "image", "qwen-vl-max").Update("provider", "tongyi-cn").Error)
	items, err := f.s.ai.ListMediaModels(f.ctx, "dev", mediaTestTenant, "qwen-cn", "", catalog.VisionUnderstanding)
	require.NoError(t, err)
	for _, item := range items {
		if item.Model == "qwen-vl-max" {
			require.True(t, item.Configured)
			require.True(t, item.Available)
			return
		}
	}
	t.Fatal("legacy vision model missing")
}
