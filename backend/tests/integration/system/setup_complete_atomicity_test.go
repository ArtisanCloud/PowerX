package systemintegration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	adminsystem "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/system"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSetupCompleteAtomicity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupInstallRuntime(t, "uninstalled")
	runtimeConfigPath := os.Getenv("POWERX_SETUP_RUNTIME_CONFIG_PATH")
	tmp := filepath.Dir(runtimeConfigPath)

	r := gin.New()
	h := adminsystem.NewSetupHandler(setupSetupDB(t))
	r.PUT("/admin/setup/config", h.SaveConfig)
	r.POST("/admin/setup/complete", h.Complete)

	payload := map[string]any{
		"deployment": map[string]any{"env": "dev"},
		"domain":     map[string]any{"domain": "powerx.local"},
		"https":      map[string]any{"mode": "auto"},
		"storage":    map[string]any{"type": "local", "local_path": "/data/uploads"},
		"cache":      map[string]any{"type": "redis", "redis_host": "127.0.0.1", "redis_port": 6379, "redis_db": 0},
		"email":      map[string]any{"enabled": false},
		"admin":      map[string]any{"username": "setup-admin", "email": "admin@example.test", "password": "SetupTestPassword123!"},
		"database": map[string]any{
			"type":        "sqlite",
			"sqlite_path": filepath.Join(tmp, "powerx.db"),
		},
		"ports": map[string]any{"backend_port": 18080, "web_admin_port": 13000},
	}
	body, _ := json.Marshal(payload)

	saveResp := httptest.NewRecorder()
	r.ServeHTTP(saveResp, httptest.NewRequest(http.MethodPut, "/admin/setup/config", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, saveResp.Code)

	t.Setenv("POWERX_SETUP_SIMULATE_PHASE2_FAIL", "true")
	failResp := httptest.NewRecorder()
	r.ServeHTTP(failResp, httptest.NewRequest(http.MethodPost, "/admin/setup/complete", bytes.NewReader([]byte(`{"licenseAccepted":true}`))))
	require.Equal(t, http.StatusInternalServerError, failResp.Code)
	require.Equal(t, "uninstalled", readInstallStatus(t, runtimeConfigPath))

	t.Setenv("POWERX_SETUP_SIMULATE_PHASE2_FAIL", "false")
	okResp := httptest.NewRecorder()
	r.ServeHTTP(okResp, httptest.NewRequest(http.MethodPost, "/admin/setup/complete", bytes.NewReader([]byte(`{"licenseAccepted":true}`))))
	require.Equal(t, http.StatusOK, okResp.Code, okResp.Body.String())
	require.Equal(t, "installed", readInstallStatus(t, runtimeConfigPath))
}

func readInstallStatus(t *testing.T, path string) string {
	t.Helper()
	raw, err := osReadFile(path)
	require.NoError(t, err)
	var root map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &root))
	install, _ := root["install"].(map[string]any)
	status, _ := install["status"].(string)
	return status
}

var osWriteFile = os.WriteFile
var osReadFile = os.ReadFile
