package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestVisionUsageUsesExplicitEvidenceAndExactOfficialIDs(t *testing.T) {
	for _, tc := range []struct{ provider, id, status, source, reason string }{
		{"openai", "gpt-5.4-mini-2026-03-17", "supported", "official_docs", ""},
		{"openai", "gpt-4o-mini", "supported", "official_docs", ""},
		{"openai", "gpt-6-luna", "supported", "official_docs", ""},
		{"openai", "tts-1", "unsupported", "official_docs", "AI_VISION_IMAGE_INPUT_UNSUPPORTED"},
		{"openai", "gpt-image-2", "unsupported", "official_docs", "AI_VISION_TEXT_OUTPUT_UNSUPPORTED"},
		{"openai", "gpt-5.4-pro-2026-03-05", "unsupported", "official_docs", "AI_VISION_PROTOCOL_UNSUPPORTED"},
		{"openai", "gpt-4o-2024-05-13", "unsupported", "official_docs", "AI_MODEL_RETIRED"},
		{"openai", "gpt-5.4-mini-future-version", "unknown", "unknown", ""},
		{"openai", "new-vision-id", "unknown", "unknown", ""},
		{"other", "tts-1", "unknown", "unknown", ""},
	} {
		t.Run(tc.provider+"/"+tc.id, func(t *testing.T) {
			usage := resolveVisionUsage(tc.provider, OnlineCatalogModel{ID: tc.id})
			require.Equal(t, tc.status, usage.Status)
			require.Equal(t, tc.source, usage.Source)
			require.Equal(t, tc.reason, usage.Reason)
			if tc.source == "official_docs" {
				require.NotEmpty(t, usage.URL)
				require.NotEmpty(t, usage.CheckedAt)
			}
		})
	}
	for _, tc := range []struct {
		input, output  []string
		status, reason string
	}{
		{[]string{"image", "text"}, []string{"text"}, "supported", ""},
		{[]string{"text"}, []string{"audio"}, "unsupported", "AI_VISION_IMAGE_INPUT_UNSUPPORTED"},
		{[]string{"image"}, []string{"image"}, "unsupported", "AI_VISION_TEXT_OUTPUT_UNSUPPORTED"},
	} {
		usage := resolveVisionUsage("other", OnlineCatalogModel{ID: "native", InputModalities: tc.input, OutputModalities: tc.output})
		require.Equal(t, tc.status, usage.Status)
		require.Equal(t, tc.reason, usage.Reason)
		require.Equal(t, "provider_api", usage.Source)
	}
	// 文本输出并不等于明确否定图片输入；部分元数据继续保留为未知。
	require.Equal(t, "unknown", resolveVisionUsage("openai", OnlineCatalogModel{ID: "new", OutputModalities: []string{"text"}, ReportedModalities: []string{"llm"}}).Status)
}

func TestVisionUsageEnrichesExistingOnlineSnapshotAndGuardsSave(t *testing.T) {
	s, _ := onlineCatalogFixture(t)
	ids := []string{"tts-1", "gpt-5.4-mini-2026-03-17", "gpt-image-2", "new-native", "gpt-5.4-pro-2026-03-05"}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		entries := []map[string]string{}
		for _, id := range ids {
			entries = append(entries, map[string]string{"id": id})
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"data": entries}))
	}))
	defer srv.Close()
	ctx, tenant := context.Background(), onlineTestTenant
	savedOnlineCredential(t, s, tenant, "dev", "openai", srv.URL+"/v1")
	synced, err := s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "dev", Provider: "openai", Modality: "vlm", Action: "refresh"})
	require.NoError(t, err)
	require.Equal(t, "unsupported", synced.Models[0].Vision.Status)
	view, err := s.ModelCatalogForTenant(ctx, "dev", &tenant, "vlm", "openai", "", "")
	require.NoError(t, err)
	require.Equal(t, ids, view.Models)
	require.Equal(t, 1, calls)
	require.Equal(t, "official_docs", view.Items[1].Vision.Source)
	require.Empty(t, view.Items[1].ReportedModalities, "official documentation must not be disguised as provider API metadata")
	for id, code := range map[string]string{"tts-1": "AI_VISION_IMAGE_INPUT_UNSUPPORTED", "gpt-image-2": "AI_VISION_TEXT_OUTPUT_UNSUPPORTED", "gpt-5.4-pro-2026-03-05": "AI_VISION_PROTOCOL_UNSUPPORTED"} {
		err := s.ValidateVLMProfile(ctx, "dev", &tenant, "openai", id, map[string]any{"max_tokens": 2048})
		require.Equal(t, code, dto.CodeOf(err))
	}
	for _, id := range []string{"gpt-5.4-mini-2026-03-17", "new-native"} {
		require.NoError(t, s.ValidateVLMProfile(ctx, "dev", &tenant, "openai", id, map[string]any{"max_tokens": 2048}))
	}
	media, err := s.MediaModelsForTenant(ctx, "dev", &tenant, "openai", "", catalog.VisionUnderstanding)
	require.NoError(t, err)
	require.Len(t, media, len(ids))
	require.Equal(t, "unsupported", media[0].Vision.Status)
}
