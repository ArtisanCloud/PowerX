package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/stretchr/testify/require"
)

func TestModelsForTenantOllamaUsesUnsavedInstanceWithoutStaticFallback(t *testing.T) {
	previous := catalog.GlobalAIRegister
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = previous })
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
	// 没有数据库：表单显式地址的只读发现不应要求保存凭据或读取其他租户。
	svc := &AgentSettingService{}
	models, err := svc.ModelsForTenant(context.Background(), "dev", nil, "llm", "Ollama (Local)", "", srv.URL)
	require.NoError(t, err)
	require.Equal(t, []string{"custom:14b"}, models)
	status = http.StatusServiceUnavailable
	models, err = svc.ModelsForTenant(context.Background(), "dev", nil, "llm", "ollama", "", srv.URL)
	require.ErrorContains(t, err, "AI_MODEL_DISCOVERY_UPSTREAM_FAILED")
	require.Nil(t, models, "failed discovery must not return YAML sample models")
}
