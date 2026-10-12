package http

import (
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestHealthPluginQuery(t *testing.T) {
	old := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = old })
	config.GlobalConfig = &config.Config{Deployment: config.DeploymentConfig{Env: "dev"}}
	for _, path := range []string{"/healthz", "/api/v1/health"} {
		for _, query := range []string{"", "?plugin_id=plugin.test", "?plugin_id=", "?plugin_id=x&plugin_id=y"} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", path+query, nil)
			HealthHandler(c)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			var response struct {
				Data      map[string]any `json:"data"`
				ErrorCode string         `json:"error_code"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			if query != "" {
				require.Equal(t, 400, w.Code)
				require.Equal(t, "HEALTH_PLUGIN_QUERY_UNSUPPORTED", response.ErrorCode)
				continue
			}
			require.Equal(t, 200, w.Code)
			require.Equal(t, "dev", response.Data["deployment_env"])
			require.Equal(t, "powerx", response.Data["runtime_mode"])
			require.Equal(t, response.Data["version"], response.Data["core_version"])
			require.NotContains(t, response.Data, "plugin_id")
			require.NotContains(t, response.Data, "plugin_version")
		}
	}
}
