package http

import (
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUnsignedSetupModeExposesNoTokenIssuingRoutes(t *testing.T) {
	cfg := config.GetDefaults()
	cfg.Auth.JWTSecret = ""
	cfg.Install = config.InstallConfig{Status: "uninstalled", AllowWithoutDB: true}
	cfg.Server.APIPrefix = "/api/v1"
	require.NoError(t, cfg.Validate())
	old := config.GlobalConfig
	config.GlobalConfig = cfg
	t.Cleanup(func() { config.GlobalConfig = old })
	engine := gin.New()
	require.NoError(t, SetupSetupOnlyRouter(cfg, engine))
	for _, route := range engine.Routes() {
		require.NotContains(t, route.Path, "/user/auth/login")
		require.NotContains(t, route.Path, "/sts/")
		require.NotContains(t, route.Path, "/tenant/invocations")
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v1/admin/setup/status", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, 200, response.Code)
	require.NotContains(t, response.Body.String(), "jwt_secret")
}
