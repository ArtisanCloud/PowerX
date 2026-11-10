package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agentsvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func ollamaModelsRouter(t *testing.T) *gin.Engine {
	t.Helper()
	previous := catalog.GlobalAIRegister
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = previous })
	h := &AgentSettingHandler{svc: agentsvc.NewAgentSettingService(nil)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), "f23491d2-b8cd-4107-a17b-301a44168aa6"))
	})
	router.GET("/models", h.listModels)
	return router
}

func TestOllamaModelsHTTPPreservesExactTagsAndDiscoveryErrors(t *testing.T) {
	router := ollamaModelsRouter(t)
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"custom:14b"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"capabilities":["completion"]}`))
		}
	}))
	defer srv.Close()
	request := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/models?env=dev&provider=ollama&modality=llm&base_url="+url.QueryEscape(srv.URL+"/v1"), nil))
		return w
	}
	w := request()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"models":["custom:14b"]`)
	status = http.StatusUnauthorized
	w = request()
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "AI_MODEL_DISCOVERY_UPSTREAM_FAILED")
	require.NotContains(t, w.Body.String(), "llama3")
}

func TestOllamaModelsHTTPLiveReadOnly(t *testing.T) {
	endpoint := os.Getenv("POWERX_OLLAMA_CATALOG_LIVE_URL")
	if endpoint == "" {
		t.Skip("set POWERX_OLLAMA_CATALOG_LIVE_URL for read-only HTTP acceptance")
	}
	router := ollamaModelsRouter(t)
	for _, modality := range []string{"llm", "embedding", "image"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/models?env=dev&provider=ollama&modality="+modality+"&base_url="+url.QueryEscape(endpoint), nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response struct {
			Data struct {
				Models []string `json:"models"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.NotNil(t, response.Data.Models)
		t.Logf("HTTP %s models: %v", modality, response.Data.Models)
	}
}
