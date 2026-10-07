package system

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func setupJWTFixture(t *testing.T) (*SetupHandler, setupConfigPayload, string) {
	t.Helper()
	t.Setenv("POWERX_AUTH_JWT_SECRET", "")
	t.Setenv("CORE_X_JWT_SECRET", "")
	if err := os.Unsetenv("CORE_X_JWT_SECRET"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CORE_X_AUTH_JWT_SECRET", "")
	if err := os.Unsetenv("CORE_X_AUTH_JWT_SECRET"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POWERX_ENV", "test")
	root := t.TempDir()
	path := filepath.Join(root, "config.yaml")
	t.Setenv("POWERX_RUNTIME_ROOT", root)
	t.Setenv("POWERX_SETUP_RUNTIME_CONFIG_PATH", path)
	require.NoError(t, os.WriteFile(path, []byte("deployment:\n  env: test\ninstall:\n  status: uninstalled\n  allow_without_db: true\nauth:\n  jwt_secret: \"\"\n"), 0600))
	old := config.GlobalConfig
	config.GlobalConfig = &config.Config{Install: config.InstallConfig{Status: "uninstalled", AllowWithoutDB: true}}
	t.Cleanup(func() { config.GlobalConfig = old })
	payload := defaultSetupConfig()
	payload.Deployment.Env = "test"
	payload.Domain.Domain = "setup.example.test"
	payload.HTTPS.Mode = "disable"
	payload.Admin.Password = "SetupTestPassword123!"
	payload.Admin.Email = "admin@example.test"
	payload.Storage.LocalPath = filepath.Join(root, "media")
	handler := NewSetupHandler(nil)
	require.NoError(t, handler.storeDraftConfig(payload))
	return handler, payload, path
}
func testPersistedJWT(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	return cfg.Auth.JWTSecret
}
func TestSetupFailureRetryRetainsSigningIdentity(t *testing.T) {
	handler, _, path := setupJWTFixture(t)
	t.Setenv("POWERX_SETUP_SIMULATE_PHASE2_FAIL", "true")
	saved := ""
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/v1/admin/setup/complete", nil)
		handler.Complete(c)
		require.Equal(t, 500, w.Code, w.Body.String())
		secret := testPersistedJWT(t, path)
		require.Len(t, secret, 64)
		if i == 0 {
			saved = secret
		} else {
			require.Equal(t, saved, secret)
		}
		require.NotContains(t, w.Body.String(), secret)
	}
}

// Child process verifies that the default migrate/seed commands see the saved
// key through the runtime path, rather than a temporary per-command override.
func TestSetupProvisionSigningKeyChild(t *testing.T) {
	if os.Getenv("POWERX_SETUP_TEST_HELPER") != "1" {
		t.Skip("child helper")
	}
	cfg, err := config.Load(os.Getenv("POWERX_CONFIG"))
	require.NoError(t, err)
	require.Len(t, cfg.Auth.JWTSecret, 64)
	digest := sha256.Sum256([]byte(cfg.Auth.JWTSecret))
	marker, err := os.OpenFile(os.Getenv("POWERX_SETUP_TEST_MARKER"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	defer marker.Close()
	_, err = fmt.Fprintln(marker, os.Getenv("POWERX_SETUP_TEST_STAGE"), hex.EncodeToString(digest[:]))
	require.NoError(t, err)
}
func TestSetupProvisionPersistsJWTForChildCommandsAndRestart(t *testing.T) {
	handler, payload, path := setupJWTFixture(t)
	root := filepath.Dir(path)
	t.Setenv("POWERX_LINKS_ROOT", root)
	toolsDir := filepath.Join(root, "backend")
	require.NoError(t, os.MkdirAll(toolsDir, 0700))
	binary, err := os.Executable()
	require.NoError(t, err)
	marker := filepath.Join(root, "stages")
	t.Setenv("POWERX_SETUP_TEST_HELPER", "1")
	t.Setenv("POWERX_SETUP_TEST_MARKER", marker)
	script := "#!/bin/sh\nexport POWERX_SETUP_TEST_STAGE=\"$1\"\nexec '" + strings.ReplaceAll(binary, "'", "'\\''") + "' -test.run='^TestSetupProvisionSigningKeyChild$'\n"
	for _, name := range []string{"database", "platform_capability_seed"} {
		require.NoError(t, os.WriteFile(filepath.Join(toolsDir, name), []byte(script), 0700))
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/admin/setup/provision", nil)
	handler.Provision(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	secret := testPersistedJWT(t, path)
	require.Len(t, secret, 64)
	require.NotContains(t, w.Body.String(), secret)
	raw, err := os.ReadFile(marker)
	require.NoError(t, err)
	rows := strings.Fields(string(raw))
	require.Len(t, rows, 6) // migrate/hash, seed/hash, platform CLI flag/hash
	require.Equal(t, "migrate", rows[0])
	require.Equal(t, "seed", rows[2])
	require.Equal(t, rows[1], rows[3])
	require.Equal(t, "-config", rows[4])
	require.Equal(t, rows[1], rows[5])
	require.NoError(t, writeRuntimeConfig(path, payload, "installed"))
	restarted, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, "installed", restarted.Install.EffectiveStatus())
	require.Equal(t, secret, restarted.Auth.JWTSecret)
}
