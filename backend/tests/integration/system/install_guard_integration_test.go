package systemintegration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corecfg "github.com/ArtisanCloud/PowerX/config"
	corehttp "github.com/ArtisanCloud/PowerX/internal/http"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInstallGuardWhitelistAndBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	cfg := &corecfg.Config{}
	cfg.Server.APIPrefix = "/api/v1"
	cfg.Install.Status = "uninstalled"
	cfg.Install.LockMode = "strict"
	r.Use(corehttp.InstallGuardMiddleware(cfg))

	r.GET("/api/v1/admin/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/api/v1/admin/deploy/releases", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	allowResp := httptest.NewRecorder()
	r.ServeHTTP(allowResp, httptest.NewRequest(http.MethodGet, "/api/v1/admin/setup/status", nil))
	require.Equal(t, http.StatusOK, allowResp.Code)

	blockResp := httptest.NewRecorder()
	r.ServeHTTP(blockResp, httptest.NewRequest(http.MethodGet, "/api/v1/admin/deploy/releases", nil))
	require.Equal(t, http.StatusServiceUnavailable, blockResp.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(blockResp.Body.Bytes(), &payload))
	require.Equal(t, "SYSTEM_NOT_INSTALLED", payload["error_code"])
}
