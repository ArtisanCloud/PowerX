package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestOnlineCatalogRepairsLegacyImageAndUnknownAssignmentsInReadAndRefresh(t *testing.T) {
	s, reg := onlineCatalogFixture(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "/v1/models", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-image-2"},{"id":"chatgpt-image-latest"},{"id":"unknown-model"},{"id":"future-image","output_modalities":["image"]},{"id":"gpt-4o"}]}`))
	}))
	defer srv.Close()
	ctx, tenant := context.Background(), onlineTestTenant
	savedOnlineCredential(t, s, tenant, "dev", "openai", srv.URL+"/v1")
	conn, err := s.savedCatalogConnection(ctx, "dev", tenant, "openai")
	require.NoError(t, err)
	legacy := storedOnlineCatalog{OnlineModelCatalog: OnlineModelCatalog{Provider: "openai", Env: "dev", Revision: "old-revision", Models: []OnlineCatalogModel{}}, Fingerprint: connectionFingerprint(conn)}
	for _, id := range []string{"gpt-image-2", "chatgpt-image-latest", "unknown-model", "future-image"} {
		legacy.Models = append(legacy.Models, OnlineCatalogModel{ID: id, Match: "manual", ManualModalities: []string{"llm"}})
	}
	raw, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, s.settingRepo.Upsert(ctx, &setting.TenantSetting{TenantUUID: tenant, Key: onlineCatalogKey("dev", "openai", ""), ValueJSON: datatypes.JSON(raw)}))
	// 型号清单不再因本地旧分类被截断；分类元数据仍在刷新时修复。
	llm, _, err := s.onlineModels(ctx, "dev", &tenant, "llm", "openai", "")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"gpt-image-2", "chatgpt-image-latest", "unknown-model", "future-image"}, llm)
	images, _, err := s.onlineModels(ctx, "dev", &tenant, "image", "openai", "")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"gpt-image-2", "chatgpt-image-latest", "unknown-model", "future-image"}, images)
	require.Zero(t, calls)

	result, err := s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "dev", Provider: "openai", Modality: "image", Action: "refresh"})
	require.NoError(t, err)
	require.Equal(t, 4, result.CorrectedCount)
	for _, i := range []int{0, 1} {
		require.Equal(t, []string{"image"}, result.Models[i].Modalities)
		require.Empty(t, result.Models[i].ManualModalities)
		require.Equal(t, "local", result.Models[i].Match)
	}
	require.True(t, result.Models[2].ReviewRequired)
	require.Equal(t, []string{"image"}, result.Models[3].ReportedModalities)
	require.Empty(t, result.Models[3].Modalities, "provider media declarations do not invent an operation contract")
	for _, id := range []string{"gpt-image-2", "gpt-image-2-2026-04-21", "chatgpt-image-latest"} {
		models, err := reg.Models("image", "openai")
		require.NoError(t, err)
		found := false
		for _, model := range models {
			if model.ID == id {
				found = true
				require.NotEmpty(t, model.Operations["image_generation"].Driver)
			}
		}
		require.True(t, found, id)
	}
	var profiles int64
	require.NoError(t, s.db.Table("ai_model_profiles").Count(&profiles).Error)
	require.Zero(t, profiles, "catalog repair must not select or save an active model")
}

func TestOnlineCatalogRejectsBulkAndConflictingAssignmentsAndSupportsExplicitRemoval(t *testing.T) {
	s, _ := onlineCatalogFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"unknown-a"},{"id":"unknown-b"},{"id":"future-image","output_modalities":["image"]}]}`))
	}))
	defer srv.Close()
	ctx, tenant := context.Background(), onlineTestTenant
	savedOnlineCredential(t, s, tenant, "dev", "openai", srv.URL+"/v1")
	result, err := s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "dev", Provider: "openai", Modality: "llm", Action: "refresh"})
	require.NoError(t, err)
	request := OnlineModelSyncRequest{Env: "dev", Provider: "openai", Modality: "llm", Action: "assign", Revision: result.Revision, Models: []string{"unknown-a"}}
	_, err = s.SyncOnlineModels(ctx, tenant, request)
	require.ErrorContains(t, err, "AI_MODEL_SYNC_CONFIRMATION_REQUIRED")
	request.ConfirmUnknown = true
	request.Models = []string{"unknown-a", "unknown-b"}
	_, err = s.SyncOnlineModels(ctx, tenant, request)
	require.ErrorContains(t, err, "AI_MODEL_SYNC_ASSIGNMENT_INVALID")
	request.Models = []string{"future-image"}
	_, err = s.SyncOnlineModels(ctx, tenant, request)
	require.ErrorContains(t, err, "AI_MODEL_SYNC_ASSIGNMENT_INVALID")
	request.Models = []string{"unknown-a"}
	assigned, err := s.SyncOnlineModels(ctx, tenant, request)
	require.NoError(t, err)
	require.True(t, assigned.Models[0].ManualConfirmed)
	refreshed, err := s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "dev", Provider: "openai", Action: "refresh"})
	require.NoError(t, err)
	require.Equal(t, "manual", refreshed.Models[0].Match)
	request.Revision, request.Action, request.ConfirmUnknown = refreshed.Revision, "unassign", false
	removed, err := s.SyncOnlineModels(ctx, tenant, request)
	require.NoError(t, err)
	require.Equal(t, "unmatched", removed.Models[0].Match)
	models, _, err := s.onlineModels(ctx, "dev", &tenant, "llm", "openai", "")
	require.NoError(t, err)
	require.Contains(t, models, "unknown-a", "legacy assignment removal does not remove a provider online ID")
}

func TestOnlineIDsAllowExplicitUsageWithoutClaimingProviderSupport(t *testing.T) {
	s, _ := onlineCatalogFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"old-configured-text"},{"id":"gpt-image-2"}]}`))
	}))
	defer srv.Close()
	ctx, tenant := context.Background(), onlineTestTenant
	savedOnlineCredential(t, s, tenant, "dev", "openai", srv.URL+"/v1")
	conn, err := s.savedCatalogConnection(ctx, "dev", tenant, "openai")
	require.NoError(t, err)
	legacy := storedOnlineCatalog{OnlineModelCatalog: OnlineModelCatalog{Provider: "openai", Env: "dev", Revision: "old", Models: []OnlineCatalogModel{
		{ID: "old-configured-text", ManualModalities: []string{"llm"}, Match: "manual"},
		{ID: "gpt-image-2", ManualModalities: []string{"llm"}, Match: "manual"},
	}}, Fingerprint: connectionFingerprint(conn)}
	raw, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, s.settingRepo.Upsert(ctx, &setting.TenantSetting{TenantUUID: tenant, Key: onlineCatalogKey("dev", "openai", ""), ValueJSON: datatypes.JSON(raw)}))
	for _, id := range []string{"old-configured-text", "gpt-image-2"} {
		require.NoError(t, s.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "llm", Provider: "openai", Model: id, Defaults: datatypes.JSONMap{}}).Error)
	}
	_, err = s.SyncOnlineModels(ctx, tenant, OnlineModelSyncRequest{Env: "dev", Provider: "openai", Action: "refresh"})
	require.NoError(t, err)
	require.NoError(t, s.ensureTenantModelExists(ctx, "dev", &tenant, "llm", "openai", "old-configured-text"))
	_, err = s.BuildModelConfig(ctx, "dev", &tenant, "llm", "openai", "old-configured-text", "", "", "", "", "", "")
	require.NoError(t, err, "review does not interrupt a previously saved unknown text profile")
	_, err = s.BuildModelConfig(ctx, "dev", &tenant, "llm", "openai", "gpt-image-2", "", "", "", "", "", "")
	require.NoError(t, err, "an online ID may be tried with the selected driver; successful inference is a separate fact")
	require.False(t, s.legacyConfiguredCandidate(ctx, "prod", &tenant, "llm", "openai", "old-configured-text"))
}
