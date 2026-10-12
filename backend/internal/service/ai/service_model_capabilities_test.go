package ai

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func modelCapabilityFixture(t *testing.T) *catalog.Registry {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fixture.yaml"), []byte(`id: fixture
name: fixture
aliases: [fixture-alias]
modalities:
  llm:
    models:
      - id: neutral-text
      - id: speech-in-name-but-text
      - id: native:tag
      - id: shared-multimodal
  audio_tts:
    models:
      - id: neutral-a
  image:
    models:
      - id: neutral-b
      - id: shared-multimodal
        capabilities: [image_generation]
      - id: declared-vision
        capabilities: [text_generation, image_understanding]
  vlm:
    models:
      - id: standalone-vision
  video:
    models:
      - id: neutral-c
apps:
  studio:
    modalities:
      llm:
        models:
          - id: app-text
          - id: shared-app-id
      audio_tts:
        models:
          - id: app-audio
  media:
    modalities:
      audio_tts:
        models:
          - id: shared-app-id
`), 0600))
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{dir}, FailIfEmpty: true}, nil))
	return reg
}

func TestTextModelProfileUsesDeclaredModalityAndRejectsMisclassifiedMedia(t *testing.T) {
	reg := modelCapabilityFixture(t)
	for _, model := range []string{"neutral-text", "speech-in-name-but-text", "app-text", "explicit-custom-text", "studio:app-text", "native:tag", "declared-vision", "standalone-vision", "studio:shared-app-id"} {
		require.True(t, textModelProfile(reg, dbModelProfileView{Modality: "llm", Provider: "fixture", Model: model}), model)
	}
	for _, model := range []string{"neutral-a", "neutral-b", "neutral-c", "app-audio", "NEUTRAL-A", "studio:app-audio", "STUDIO:APP-AUDIO", "media:shared-app-id", "shared-app-id"} {
		require.False(t, textModelProfile(reg, dbModelProfileView{Modality: "llm", Provider: "fixture", Model: model}), model)
	}
	for _, modality := range []string{"", "image", "video", "audio_tts", "vlm"} {
		require.False(t, textModelProfile(reg, dbModelProfileView{Modality: modality, Provider: "fixture", Model: "neutral-text"}), modality)
	}
}

func TestTextCatalogPreservesApplicationsAndMergesDeclaredCapabilities(t *testing.T) {
	items := textCatalogModels(modelCapabilityFixture(t), "fixture-alias")
	models := map[string]catalog.ModelManifest{}
	for _, item := range items {
		require.NotContains(t, models, item.ID)
		models[item.ID] = item
	}
	require.Contains(t, models, "studio:app-text")
	require.NotContains(t, models, "app-text")
	require.NotContains(t, models, "studio:app-audio")
	require.NotContains(t, models, "media:shared-app-id")
	require.ElementsMatch(t, []string{"text_generation", "image_generation"}, models["shared-multimodal"].Capabilities)
	require.ElementsMatch(t, []string{"text_generation", "image_understanding"}, models["declared-vision"].Capabilities)
}

func TestExistingVisionCatalogKeepsTextGenerationDeclarations(t *testing.T) {
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	for _, in := range []dbModelProfileView{
		{Modality: "llm", Provider: "qwen", Model: "qwen-vl-max"},
		{Modality: "llm", Provider: "qwen-cn", Model: "qwen3-vl-flash"},
		{Modality: "llm", Provider: "ollama", Model: "phi3:vision"},
	} {
		capabilities, accepted := textModelProfileCapabilities(reg, in)
		require.True(t, accepted, in)
		require.Contains(t, capabilities, "text_generation")
		require.Contains(t, capabilities, "image_understanding")
	}
	for _, in := range []dbModelProfileView{
		{Modality: "llm", Provider: "qwen", Model: "qwen-tts-latest"},
		{Modality: "llm", Provider: "google", Model: "gemini-2.5-flash-image"},
	} {
		require.False(t, textModelProfile(reg, in), in)
	}
}

func TestListLLMModelsReturnsDeclaredMetadataAndScopedProfiles(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	for _, statement := range []string{
		`CREATE TABLE ai_provider_credentials(id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime, env text, tenant_uuid text, name text, provider text, auth_scheme text, data text)`,
		`CREATE TABLE ai_model_profiles(id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime, env text, tenant_uuid text, modality text, provider text, model text, label text, defaults text, cap_cache text, tags text)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	tenant, other := uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Create(&dbmodel.AIProviderCredential{Env: "dev", TenantUUID: &tenant, Name: "test", Provider: "fixture-alias", Data: datatypes.JSONMap{"api_key": "synthetic-only"}}).Error)
	for _, profile := range []dbmodel.AIModelProfile{
		{Env: "dev", Modality: "llm", Provider: "fixture", Model: "neutral-text", Label: "Global"},
		{Env: "dev", TenantUUID: &tenant, Modality: "llm", Provider: "fixture-alias", Model: "neutral-text", Label: "Tenant", Defaults: datatypes.JSONMap{"temperature": 0.2}},
		{Env: "dev", TenantUUID: &tenant, Modality: "llm", Provider: "fixture", Model: "studio:app-audio"},
		{Env: "dev", TenantUUID: &tenant, Modality: "llm", Provider: "fixture", Model: "declared-vision"},
		{Env: "dev", TenantUUID: &tenant, Modality: "llm", Provider: "fixture", Model: "custom-model"},
		{Env: "dev", TenantUUID: &other, Modality: "llm", Provider: "fixture", Model: "private-other-tenant"},
		{Env: "prod", TenantUUID: &tenant, Modality: "llm", Provider: "fixture", Model: "private-other-env"},
	} {
		require.NoError(t, db.Create(&profile).Error)
	}
	svc := NewService(db)
	svc.catalog = modelCapabilityFixture(t)
	items, err := svc.ListLLMModels(context.Background(), "dev", tenant, "fixture-alias")
	require.NoError(t, err)
	byID := map[string]LLMModelItem{}
	for _, item := range items {
		require.NotContains(t, byID, item.Model)
		byID[item.Model] = item
		require.Equal(t, "llm", item.Modality)
		require.Contains(t, item.Capabilities, "text_generation")
		require.True(t, item.Configured)
	}
	require.Equal(t, "Tenant", byID["neutral-text"].Label)
	require.Equal(t, "tenant_profile", byID["neutral-text"].Source)
	require.Equal(t, json.Number("0.2"), byID["neutral-text"].Defaults["temperature"])
	require.Equal(t, "tenant_profile", byID["custom-model"].Source)
	require.Equal(t, "provider_catalog", byID["studio:app-text"].Source)
	require.False(t, byID["studio:app-text"].ProfileConfigured)
	require.ElementsMatch(t, []string{"text_generation", "image_understanding"}, byID["declared-vision"].Capabilities)
	for _, excluded := range []string{"studio:app-audio", "neutral-b", "neutral-c", "private-other-tenant", "private-other-env"} {
		require.NotContains(t, byID, excluded)
	}
	encoded, err := json.Marshal(items)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"modality":"llm"`)
	require.Contains(t, string(encoded), `"capabilities":[`)
	require.NotContains(t, string(encoded), "synthetic-only")
}
