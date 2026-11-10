package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestOnlineFirstCatalogCompleteAndUsableWithoutLocalModelRegistration(t *testing.T) {
	s, reg := onlineCatalogFixture(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/models", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":[{"id":"fresh-vision-native"},{"id":"typed-vision","input_modalities":["text","image"],"output_modalities":["text"]}]}`))
	}))
	defer srv.Close()
	ctx, tenant := context.Background(), onlineTestTenant
	savedOnlineCredential(t, s, tenant, "dev", "openai", srv.URL+"/v1")
	view, err := s.ModelCatalogForTenant(ctx, "dev", &tenant, "vlm", "openai", "", "")
	require.NoError(t, err)
	require.Equal(t, "online", view.Source)
	require.Equal(t, []string{"fresh-vision-native", "typed-vision"}, view.Models)
	require.NotContains(t, view.Models, "gpt-6-luna", "local examples cannot be merged into an authoritative online list")
	require.Empty(t, view.Items[0].ReportedModalities)
	require.Contains(t, view.Items[1].ReportedModalities, "vlm")
	require.NoError(t, s.ValidateVLMProfile(ctx, "dev", &tenant, "openai", "fresh-vision-native", map[string]any{"max_tokens": 2048}))
	model, err := s.TenantModelManifest(ctx, "dev", &tenant, "vlm", "openai", "fresh-vision-native")
	require.NoError(t, err)
	require.Equal(t, "openai", model.Operations[catalog.VisionUnderstanding].Driver)
	require.Nil(t, findModelManifest("vlm", "openai", "fresh-vision-native"))
	local, _ := reg.Models("vlm", "openai")
	require.Len(t, local, 1, "online tenant models never mutate the global manifest")
	require.Equal(t, 1, calls, "subsequent reads reuse the stored online snapshot")
	_, err = s.TenantModelManifest(ctx, "dev", &tenant, "vlm", "openai", "gpt-6-luna")
	require.ErrorContains(t, err, "AI_MODEL_NOT_IN_ONLINE_CATALOG")
}

func TestOnlineFirstFailuresDoNotFallBackToLocalExamples(t *testing.T) {
	s, _ := onlineCatalogFixture(t)
	status := http.StatusUnauthorized
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
	defer srv.Close()
	ctx, tenant := context.Background(), onlineTestTenant
	savedOnlineCredential(t, s, tenant, "dev", "openai", srv.URL+"/v1")
	for _, code := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusBadGateway} {
		status = code
		models, err := s.ModelsForTenant(ctx, "dev", &tenant, "vlm", "openai", "", "")
		require.Error(t, err)
		require.Empty(t, models)
	}
	models, err := s.ModelsForTenant(ctx, "dev", &tenant, "image", "agnes", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, models, "providers with no declared model API keep their explicit local catalog")
}

func TestDeploymentWithoutModelAPIUsesExplicitLocalCatalog(t *testing.T) {
	s, _ := onlineCatalogFixture(t)
	tenant, ctx := onlineTestTenant, context.Background()
	data, err := s.tks.SealSensitive(ctx, "dev", tenant, datatypes.JSONMap{"base_url": "https://deployment.example.invalid", "api_key": "fixture-only", "azure_deployment": "fixture-deployment"}, "api_key")
	require.NoError(t, err)
	require.NoError(t, s.db.Create(&am.AIProviderCredential{Env: "dev", TenantUUID: &tenant, Name: "dev-openai", Provider: "openai", Data: data}).Error)
	view, err := s.ModelCatalogForTenant(ctx, "dev", &tenant, "llm", "openai", "", "")
	require.NoError(t, err)
	require.Equal(t, "local", view.Source)
	require.NotEmpty(t, view.Models)
}
