package systemintegration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	corecfg "github.com/ArtisanCloud/PowerX/config"
	adminsystem "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/system"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSetupCompleteAtomicity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	runtimeConfigPath := filepath.Join(tmp, "config.yaml")
	draftPath := filepath.Join(tmp, "setup.wizard.config.json")
	require.NoError(t, osWriteFile(runtimeConfigPath, []byte("version: v1.0.0\nserver:\n  port: 8077\ninstall:\n  status: uninstalled\n  lock_mode: strict\n  allow_without_db: true\n"), 0o644))

	t.Setenv("POWERX_SETUP_RUNTIME_CONFIG_PATH", runtimeConfigPath)
	t.Setenv("POWERX_SETUP_DRAFT_PATH", draftPath)

	corecfg.GlobalConfig = &corecfg.Config{}
	corecfg.GlobalConfig.Install.Status = "uninstalled"
	corecfg.GlobalConfig.Install.LockMode = "strict"

	r := gin.New()
	h := adminsystem.NewSetupHandler(nil)
	r.PUT("/admin/setup/config", h.SaveConfig)
	r.POST("/admin/setup/complete", h.Complete)

	payload := map[string]any{
		"domain":  map[string]any{"domain": "powerx.local"},
		"https":   map[string]any{"mode": "auto"},
		"storage": map[string]any{"type": "local", "local_path": "/data/uploads"},
		"cache":   map[string]any{"type": "redis", "redis_host": "127.0.0.1", "redis_port": 6379, "redis_db": 0},
		"email":   map[string]any{"enabled": false},
		"ports":   map[string]any{"backend_port": 18080, "web_admin_port": 13000},
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
	require.Equal(t, http.StatusOK, okResp.Code)
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
