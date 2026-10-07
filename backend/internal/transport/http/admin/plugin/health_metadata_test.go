package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type healthMetadataManager struct {
	plugin_mgr.Manager
	err       error
	runtimeOK bool
}

func (m healthMetadataManager) Get(context.Context, string) (plugin_mgr.Plugin, error) {
	return plugin_mgr.Plugin{Version: "1.2.3"}, m.err
}
func (m healthMetadataManager) RuntimeStatus(string) (supervisor.ProcInfo, bool) {
	return supervisor.ProcInfo{State: supervisor.ProcRunning}, m.runtimeOK
}
func TestAdminRuntimeIdentityRoute(t *testing.T) {
	old := config.GlobalConfig
	config.GlobalConfig = &config.Config{Deployment: config.DeploymentConfig{Env: "dev"}}
	t.Cleanup(func() { config.GlobalConfig = old; mgrimpl.ResetGlobalForTest(nil) })
	root := false
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithIsRoot(c.Request.Context(), root))
		c.Next()
	})
	RegisterAPIRoutes(engine.Group("/api/v1"), engine.Group("/api/v1"), &shared.Deps{})
	call := func(status int, code string) {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/admin/plugins/plugin.test/runtime-identity", nil)
		engine.ServeHTTP(w, req)
		require.Equal(t, status, w.Code)
		if root {
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		}
		var body struct {
			ErrorCode string `json:"error_code"`
			Data      struct {
				Item struct {
					Version string `json:"plugin_version"`
					Source  string `json:"plugin_version_source"`
				} `json:"item"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		if code != "" {
			require.Equal(t, code, body.ErrorCode)
		}
		if status == 200 {
			require.Equal(t, "1.2.3", body.Data.Item.Version)
			require.Equal(t, "registry", body.Data.Item.Source)
		}
	}
	call(403, "") // Existing Admin role guard is required before manager access.
	root = true
	mgrimpl.ResetGlobalForTest(healthMetadataManager{runtimeOK: true})
	call(200, "")
	mgrimpl.ResetGlobalForTest(healthMetadataManager{})
	call(503, "RUNTIME_IDENTITY_STATE_UNAVAILABLE")
	mgrimpl.ResetGlobalForTest(healthMetadataManager{err: errors.New("IO failure")})
	call(503, "RUNTIME_IDENTITY_UNAVAILABLE")
	mgrimpl.ResetGlobalForTest(healthMetadataManager{err: plugin_mgr.NewError(plugin_mgr.CodeNotFound)})
	call(404, "RUNTIME_IDENTITY_PLUGIN_NOT_FOUND")
}
