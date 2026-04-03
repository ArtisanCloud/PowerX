package systemintegration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetupStatusDesiredEffectivePortsAndRestartRequired(t *testing.T) {
	setupInstallRuntime(t, "uninstalled")
	db := setupSetupDB(t)
	router := setupSetupRouter(db)

	savePayload := map[string]any{
		"domain": map[string]any{
			"domain": "powerx.local",
		},
		"https": map[string]any{
			"mode":       "auto",
			"cert_email": "ops@powerx.local",
		},
		"storage": map[string]any{
			"type":       "local",
			"local_path": "/data/uploads",
		},
		"cache": map[string]any{
			"type":       "redis",
			"redis_host": "127.0.0.1",
			"redis_port": 6379,
			"redis_db":   0,
		},
		"email": map[string]any{
			"enabled": false,
		},
		"database": map[string]any{
			"type":        "postgresql",
			"host":        "localhost",
			"port":        5432,
			"name":        "powerx",
			"username":    "postgres",
			"password":    "postgres",
			"ssl_mode":    "disable",
			"sqlite_path": "/tmp/powerx.db",
		},
		"ports": map[string]any{
			"backend_port":   18080,
			"web_admin_port": 13000,
		},
	}
	saveResp := performSetupRequest(t, router, http.MethodPut, "/admin/setup/config", savePayload)
	require.Equal(t, http.StatusOK, saveResp.Code)

	statusResp := performSetupRequest(t, router, http.MethodGet, "/admin/setup/status", nil)
	require.Equal(t, http.StatusOK, statusResp.Code)
	statusData := mustReadStatusData(t, statusResp.Body.Bytes())
	require.Equal(t, true, statusData["restart_required"])

	desired := mustReadPortsMap(t, statusData, "desired_ports")
	effective := mustReadPortsMap(t, statusData, "effective_ports")
	require.Equal(t, float64(18080), desired["backend_port"])
	require.Equal(t, float64(13000), desired["web_admin_port"])
	require.Equal(t, float64(8077), effective["backend_port"])
	require.Equal(t, float64(3000), effective["web_admin_port"])

	configSource, ok := statusData["config_source"].(map[string]any)
	require.True(t, ok, "config_source should be object")
	require.NotEmpty(t, configSource["desired_ports"])
	require.NotEmpty(t, configSource["effective_ports"])

	runtimeConfigPath := os.Getenv("POWERX_SETUP_RUNTIME_CONFIG_PATH")
	require.NotEmpty(t, runtimeConfigPath)
	runtimeText := "version: v1.0.0\nserver:\n  port: 18080\ninstall:\n  status: uninstalled\n  lock_mode: strict\n  allow_without_db: true\n"
	require.NoError(t, os.WriteFile(runtimeConfigPath, []byte(runtimeText), 0o644))
	t.Setenv("POWERX_WEB_ADMIN_PORT", "13000")

	statusResp2 := performSetupRequest(t, router, http.MethodGet, "/admin/setup/status", nil)
	require.Equal(t, http.StatusOK, statusResp2.Code)
	statusData2 := mustReadStatusData(t, statusResp2.Body.Bytes())
	require.Equal(t, false, statusData2["restart_required"])

	effective2 := mustReadPortsMap(t, statusData2, "effective_ports")
	require.Equal(t, float64(18080), effective2["backend_port"])
	require.Equal(t, float64(13000), effective2["web_admin_port"])
}

func mustReadStatusData(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok, fmt.Sprintf("response data missing: %s", string(body)))
	return data
}

func mustReadPortsMap(t *testing.T, data map[string]any, key string) map[string]any {
	t.Helper()
	raw, ok := data[key].(map[string]any)
	require.True(t, ok, "%s should be object", key)
	return raw
}
