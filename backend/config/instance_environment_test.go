package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func isolateInstanceEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("POWERX_ENV", "test")
	for _, key := range []string{"POWERX_AUTH_JWT_SECRET", "CORE_X_AUTH_JWT_SECRET", "CORE_X_JWT_SECRET", "CORE_X_DB_DATABASE", "CORE_X_DB_PASSWORD", "POWERX_CONFIG", "POWERX_UPGRADE_MODE"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}

func writeInstanceEnvironmentFixture(t *testing.T, directory, environment, key string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(directory, 0700))
	path := filepath.Join(directory, "config.yaml")
	body := "deployment:\n  env: " + environment + "\ninstall:\n  status: installed\nauth:\n  jwt_secret: " + legacyDefaultJWTSecret + "\ndatabase:\n  database: original\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))
	file := "POWERX_AUTH_JWT_SECRET=\"" + key + "\"\nCORE_X_DB_DATABASE=" + environment + "_database\nCORE_X_DB_PASSWORD='literal$PASSWORD'\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, "powerx.env"), []byte(file), 0600))
	return path
}

func TestInstanceEnvironmentLoadsProdAndDevWithoutGlobalMutation(t *testing.T) {
	isolateInstanceEnvironment(t)
	root := t.TempDir()
	prodKey, devKey := randomTestJWTSecret(t), randomTestJWTSecret(t)
	prod := writeInstanceEnvironmentFixture(t, filepath.Join(root, "powerx"), "prod", prodKey)
	dev := writeInstanceEnvironmentFixture(t, filepath.Join(root, "powerx-dev"), "dev", devKey)
	for i := 0; i < 2; i++ {
		for _, row := range []struct{ path, key, environment string }{{prod, prodKey, "prod"}, {dev, devKey, "dev"}} {
			before, err := os.ReadFile(row.path)
			require.NoError(t, err)
			cfg, err := Load(row.path)
			require.NoError(t, err)
			require.Equal(t, row.key, cfg.Auth.JWTSecret)
			require.Equal(t, row.environment, cfg.Deployment.Env)
			require.Equal(t, row.environment+"_database", cfg.Database.Database)
			require.Equal(t, "literal$PASSWORD", cfg.Database.Password)
			after, err := os.ReadFile(row.path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		}
	}
	_, present := os.LookupEnv("POWERX_AUTH_JWT_SECRET")
	require.False(t, present)
	_, present = os.LookupEnv("CORE_X_DB_DATABASE")
	require.False(t, present)
	var wg sync.WaitGroup
	for _, row := range []struct{ path, key string }{{prod, prodKey}, {dev, devKey}} {
		wg.Add(1)
		go func(path, key string) {
			defer wg.Done()
			cfg, err := Load(path)
			require.NoError(t, err)
			if err == nil {
				require.Equal(t, key, cfg.Auth.JWTSecret)
			}
		}(row.path, row.key)
	}
	wg.Wait()
}

func TestInstanceEnvironmentRetainsExplicitProcessPrecedenceAndStrictJWTValidation(t *testing.T) {
	isolateInstanceEnvironment(t)
	key := randomTestJWTSecret(t)
	path := writeInstanceEnvironmentFixture(t, t.TempDir(), "prod", key)
	processKey := randomTestJWTSecret(t)
	t.Setenv("POWERX_AUTH_JWT_SECRET", processKey)
	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, processKey, cfg.Auth.JWTSecret)
	t.Setenv("POWERX_AUTH_JWT_SECRET", legacyDefaultJWTSecret)
	_, err = Load(path)
	require.ErrorContains(t, err, "禁止使用公开默认值")
	require.NotContains(t, err.Error(), legacyDefaultJWTSecret)
	require.NoError(t, os.Unsetenv("POWERX_AUTH_JWT_SECRET"))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), "powerx.env"), []byte("POWERX_AUTH_JWT_SECRET="+legacyDefaultJWTSecret+"\n"), 0600))
	_, err = Load(path)
	require.ErrorContains(t, err, "禁止使用公开默认值")
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), "powerx.env"), []byte("CORE_X_AUTH_JWT_SECRET="+key+"\n"), 0600))
	_, err = Load(path)
	require.ErrorContains(t, err, "已不再支持")
	require.NotContains(t, err.Error(), key)
}

func TestInstanceEnvironmentRejectsMalformedOrUnreadableFilesWithoutSecretDisclosure(t *testing.T) {
	isolateInstanceEnvironment(t)
	key := randomTestJWTSecret(t)
	path := writeInstanceEnvironmentFixture(t, t.TempDir(), "dev", key)
	for _, body := range []string{"POWERX_AUTH_JWT_SECRET=\"" + key + "\n", key + "\n"} {
		require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), "powerx.env"), []byte(body), 0600))
		_, err := Load(path)
		require.Error(t, err)
		require.NotContains(t, err.Error(), key)
	}
	require.NoError(t, os.Remove(filepath.Join(filepath.Dir(path), "powerx.env")))
	require.NoError(t, os.Mkdir(filepath.Join(filepath.Dir(path), "powerx.env"), 0700))
	_, err := Load(path)
	require.ErrorContains(t, err, "不是普通文件")
}

func TestSetupUsesExistingInstanceEnvironmentSigningKeyWithoutRotating(t *testing.T) {
	isolateInstanceEnvironment(t)
	key := randomTestJWTSecret(t)
	path := writeInstanceEnvironmentFixture(t, t.TempDir(), "dev", key)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, PrepareSetupJWTSecret(path))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, key, cfg.Auth.JWTSecret)
	require.True(t, strings.Contains(string(after), legacyDefaultJWTSecret), "loader and setup do not rewrite operator configuration")
}
