package systemintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corecfg "github.com/ArtisanCloud/PowerX/config"
	adminsystem "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/system"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modelsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSetupBootstrapFlow(t *testing.T) {
	setupInstallRuntime(t, "uninstalled")
	db := setupSetupDB(t)
	router := setupSetupRouter(db)

	// 1) 初始状态：未配置，且不需要登录
	statusResp := performSetupRequest(t, router, http.MethodGet, "/admin/setup/status", nil)
	require.Equal(t, http.StatusOK, statusResp.Code)
	require.Contains(t, statusResp.Body.String(), `"configured":false`)
	require.Contains(t, statusResp.Body.String(), `"requires_login":false`)

	// 2) 写入配置草稿
	payload := map[string]any{
		"deployment": map[string]any{"env": "dev"},
		"domain": map[string]any{
			"domain":        "powerx.local",
			"api_subdomain": "api",
			"enable_cdn":    false,
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
			"type":        "sqlite",
			"sqlite_path": filepath.Join(t.TempDir(), "powerx.db"),
		},
		"ports": map[string]any{
			"backend_port":   18080,
			"web_admin_port": 13000,
		},
	}
	saveResp := performSetupRequest(t, router, http.MethodPut, "/admin/setup/config", payload)
	require.Equal(t, http.StatusOK, saveResp.Code)
	require.Contains(t, saveResp.Body.String(), `"ok":true`)

	// 3) 回显配置草稿
	cfgResp := performSetupRequest(t, router, http.MethodGet, "/admin/setup/config", nil)
	require.Equal(t, http.StatusOK, cfgResp.Code)
	require.Contains(t, cfgResp.Body.String(), `"domain":"powerx.local"`)
	require.Contains(t, cfgResp.Body.String(), `"deployment":{"env":"dev"}`)
	require.Contains(t, cfgResp.Body.String(), `"backend_port":18080`)
	require.Contains(t, cfgResp.Body.String(), `"web_admin_port":13000`)

	// 4) 完成初始化
	completeResp := performSetupRequest(t, router, http.MethodPost, "/admin/setup/complete", map[string]any{"licenseAccepted": true})
	require.Equal(t, http.StatusOK, completeResp.Code, completeResp.Body.String())
	require.Contains(t, completeResp.Body.String(), `"configured":true`)

	// 5) 状态变为 configured=true
	statusResp2 := performSetupRequest(t, router, http.MethodGet, "/admin/setup/status", nil)
	require.Equal(t, http.StatusOK, statusResp2.Code)
	require.Contains(t, statusResp2.Body.String(), `"configured":true`)
	runtimeConfig, err := os.ReadFile(os.Getenv("POWERX_SETUP_RUNTIME_CONFIG_PATH"))
	require.NoError(t, err)
	require.Contains(t, string(runtimeConfig), "deployment:\n    env: dev")
}

func TestSetupSaveConfigRejectInvalidPayload(t *testing.T) {
	setupInstallRuntime(t, "uninstalled")
	db := setupSetupDB(t)
	router := setupSetupRouter(db)

	invalidPayload := map[string]any{
		"deployment": map[string]any{"env": "dev"},
		"domain": map[string]any{
			"domain": "",
		},
		"https": map[string]any{
			"mode": "manual",
		},
		"storage": map[string]any{
			"type": "local",
		},
		"cache": map[string]any{
			"type": "redis",
		},
		"email": map[string]any{
			"enabled": false,
		},
	}
	resp := performSetupRequest(t, router, http.MethodPut, "/admin/setup/config", invalidPayload)
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestSetupSaveConfigRequiresValidDeploymentEnv(t *testing.T) {
	setupInstallRuntime(t, "uninstalled")
	db := setupSetupDB(t)
	router := setupSetupRouter(db)

	basePayload := map[string]any{
		"deployment": map[string]any{"env": ""},
		"domain":     map[string]any{"domain": "powerx.local"},
		"https":      map[string]any{"mode": "auto"},
		"storage":    map[string]any{"type": "local", "local_path": "/data/uploads"},
		"cache":      map[string]any{"type": "redis", "redis_host": "127.0.0.1", "redis_port": 6379},
		"email":      map[string]any{"enabled": false},
		"ports":      map[string]any{"backend_port": 8080, "web_admin_port": 3000},
	}

	missing := performSetupRequest(t, router, http.MethodPut, "/admin/setup/config", basePayload)
	require.Equal(t, http.StatusBadRequest, missing.Code)
	require.Contains(t, missing.Body.String(), "setup.errors.deploymentEnvRequired")

	basePayload["deployment"] = map[string]any{"env": "production"}
	invalid := performSetupRequest(t, router, http.MethodPut, "/admin/setup/config", basePayload)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.Contains(t, invalid.Body.String(), "setup.errors.deploymentEnvInvalid")
}

func TestSetupCompleteWhenAlreadyInstalled(t *testing.T) {
	setupInstallRuntime(t, "installed")
	t.Setenv("POWERX_ALLOW_SETUP_REENTRY", "true")
	db := setupSetupDB(t)

	router := setupSetupRouter(db)
	resp := performSetupRequest(t, router, http.MethodPost, "/admin/setup/complete", map[string]any{"licenseAccepted": true})
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"configured":true`)
}

func TestSetupConfigPortDefaultsAndEnvOverride(t *testing.T) {
	setupInstallRuntime(t, "uninstalled")
	db := setupSetupDB(t)
	router := setupSetupRouter(db)

	t.Setenv("POWERX_ENV", "dev")
	respDev := performSetupRequest(t, router, http.MethodGet, "/admin/setup/config", nil)
	require.Equal(t, http.StatusOK, respDev.Code)
	require.Contains(t, respDev.Body.String(), `"backend_port":8077`)
	require.Contains(t, respDev.Body.String(), `"web_admin_port":3030`)

	t.Setenv("POWERX_ENV", "prod")
	t.Setenv("POWERX_BACKEND_PORT", "18088")
	t.Setenv("POWERX_WEB_ADMIN_PORT", "13088")
	respEnv := performSetupRequest(t, router, http.MethodGet, "/admin/setup/config", nil)
	require.Equal(t, http.StatusOK, respEnv.Code)
	require.Contains(t, respEnv.Body.String(), `"backend_port":18088`)
	require.Contains(t, respEnv.Body.String(), `"web_admin_port":13088`)
}

func TestSetupSaveConfigRejectInvalidPorts(t *testing.T) {
	setupInstallRuntime(t, "uninstalled")
	db := setupSetupDB(t)
	router := setupSetupRouter(db)

	payload := map[string]any{
		"deployment": map[string]any{"env": "dev"},
		"domain": map[string]any{
			"domain": "powerx.local",
		},
		"https": map[string]any{
			"mode": "auto",
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
			"type":        "sqlite",
			"sqlite_path": filepath.Join(t.TempDir(), "powerx.db"),
		},
		"ports": map[string]any{
			"backend_port":   8080,
			"web_admin_port": 8080,
		},
	}

	resp := performSetupRequest(t, router, http.MethodPut, "/admin/setup/config", payload)
	require.Equal(t, http.StatusBadRequest, resp.Code)
	require.Contains(t, resp.Body.String(), "不能相同")
}

func TestSetupStatusSupportsLegacyInstalledKeys(t *testing.T) {
	setupInstallRuntime(t, "uninstalled")
	db := setupSetupDB(t)
	require.NoError(t, db.Create(&modelsetting.SystemSetting{
		Key:       "platform.installed",
		ValueJSON: []byte("true"),
		Group:     "setup",
		Editable:  true,
	}).Error)

	router := setupSetupRouter(db)
	statusResp := performSetupRequest(t, router, http.MethodGet, "/admin/setup/status", nil)
	require.Equal(t, http.StatusOK, statusResp.Code)
	require.Contains(t, statusResp.Body.String(), `"configured":false`)
	require.Contains(t, statusResp.Body.String(), `"completed_by_kv":true`)
}

func setupInstallRuntime(t *testing.T, status string) {
	t.Helper()
	tmp := t.TempDir()
	runtimeConfigPath := filepath.Join(tmp, "config.yaml")
	draftPath := filepath.Join(tmp, "setup.wizard.config.json")
	content := fmt.Sprintf("version: v1.0.0\ndeployment:\n  env: dev\nserver:\n  port: 8077\ninstall:\n  status: %s\n  lock_mode: strict\n  allow_without_db: true\n", status)
	require.NoError(t, os.WriteFile(runtimeConfigPath, []byte(content), 0o644))
	t.Setenv("POWERX_SETUP_RUNTIME_CONFIG_PATH", runtimeConfigPath)
	t.Setenv("POWERX_SETUP_DRAFT_PATH", draftPath)
	t.Setenv("POWERX_SETUP_SIMULATE_PHASE2_FAIL", "false")
	corecfg.GlobalConfig = &corecfg.Config{}
	corecfg.GlobalConfig.Version = "v1.0.0"
	corecfg.GlobalConfig.Deployment.Env = "dev"
	corecfg.GlobalConfig.Server.APIPrefix = "/api/v1"
	corecfg.GlobalConfig.Install.Status = status
	corecfg.GlobalConfig.Install.LockMode = "strict"
}

func setupSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&parseTime=true&_loc=UTC", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&modeliam.User{},
		&modeltenant.Tenant{},
		&modelsetting.SystemSetting{},
	))
	return db
}

func setupSetupRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := adminsystem.NewSetupHandler(db)
	r.GET("/admin/setup/status", h.Status)
	r.GET("/admin/setup/config", h.GetConfig)
	r.PUT("/admin/setup/config", h.SaveConfig)
	r.POST("/admin/setup/complete", h.Complete)
	return r
}

func performSetupRequest(t *testing.T, r http.Handler, method, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
