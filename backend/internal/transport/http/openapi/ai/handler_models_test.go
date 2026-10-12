package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLLMModelCatalogHTTPRetainsDeclaredCapabilitiesAndEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant := "11111111-1111-4111-8111-111111111111"
	h := &aiHandler{svc: &stubAIService{llmModelsFn: func(ctx context.Context, env, actualTenant, provider string) ([]aisvc.LLMModelItem, error) {
		require.Equal(t, tenant, actualTenant)
		require.Equal(t, tenant, reqctx.GetTenantUUID(ctx))
		require.Equal(t, "dev", env)
		require.Equal(t, "fixture-alias", provider)
		return []aisvc.LLMModelItem{
			{ModelKey: "fixture/studio:text", Modality: "llm", Capabilities: []string{"text_generation"}, Provider: "fixture", Model: "studio:text", Source: "provider_catalog", Configured: true},
			{ModelKey: "fixture/vision", Modality: "llm", Capabilities: []string{"text_generation", "image_understanding"}, Provider: "fixture", Model: "vision", Source: "tenant_profile", Configured: false, ProfileConfigured: true},
		}, nil
	}}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithEnv(reqctx.WithTenantUUID(c.Request.Context(), tenant), "dev"))
	})
	router.GET("/api/v1/ai/llm/models", h.llmModelsList)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/ai/llm/models?provider=fixture-alias", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Data struct {
			Env   string               `json:"env"`
			Items []aisvc.LLMModelItem `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "dev", body.Data.Env)
	require.Len(t, body.Data.Items, 2)
	require.Equal(t, "llm", body.Data.Items[0].Modality)
	require.Equal(t, "studio:text", body.Data.Items[0].Model)
	require.Equal(t, []string{"text_generation", "image_understanding"}, body.Data.Items[1].Capabilities)
	require.False(t, body.Data.Items[1].Configured)
	require.True(t, body.Data.Items[1].ProfileConfigured)
}
