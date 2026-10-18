package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func isolateCommandConfiguration(t *testing.T) {
	t.Helper()
	t.Setenv("POWERX_ENV", "test")
	for _, name := range []string{"POWERX_CONFIG", "POWERX_AUTH_JWT_SECRET", "CORE_X_AUTH_JWT_SECRET", "CORE_X_JWT_SECRET", "POWERX_UPGRADE_MODE"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}
}

func TestInitSecretsCommandWorksOfflineAndDoesNotExposeSecrets(t *testing.T) {
	isolateCommandConfiguration(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("deployment:\n  env: test\ninstall:\n  status: uninstalled\ndatabase:\n  host: invalid.invalid\nauth:\n  jwt_secret: \"\"\nserver:\n  secret_key: \"\"\n"), 0600))
	t.Setenv("POWERX_CONFIG", path)
	var output, errorOutput bytes.Buffer
	handled, code := handleInitSecrets([]string{"init-secrets"}, &output, &errorOutput)
	require.True(t, handled)
	require.Zero(t, code, errorOutput.String())
	require.Contains(t, output.String(), "未连接数据库")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg struct {
		Auth struct {
			JWT string `yaml:"jwt_secret"`
		} `yaml:"auth"`
		Server struct {
			Key string `yaml:"secret_key"`
		} `yaml:"server"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	require.Len(t, cfg.Auth.JWT, 64)
	require.NotContains(t, output.String()+errorOutput.String(), cfg.Auth.JWT)
	require.NotContains(t, output.String()+errorOutput.String(), cfg.Server.Key)
	handled, code = handleInitSecrets([]string{"init-secrets", "-config", path}, &output, &errorOutput)
	require.True(t, handled)
	require.Zero(t, code)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, raw, after)
	t.Setenv("POWERX_UPGRADE_MODE", "1")
	_, code = handleInitSecrets([]string{"init-secrets"}, &output, &errorOutput)
	require.NotZero(t, code)
	after, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, raw, after)
}

func TestInitSecretsHelpAndBadFlagsDoNotInitialize(t *testing.T) {
	isolateCommandConfiguration(t)
	for _, args := range [][]string{{"init-secrets", "--help"}, {"init-secrets", "-unknown"}, {"init-secrets", "extra"}} {
		var output, errorOutput bytes.Buffer
		handled, code := handleInitSecrets(args, &output, &errorOutput)
		require.True(t, handled)
		if args[1] == "--help" {
			require.Zero(t, code)
		} else {
			require.NotZero(t, code)
		}
	}
}

func TestDatabaseCommandLimitsMigrationValidationToMigrationAndStatus(t *testing.T) {
	isolateCommandConfiguration(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("deployment:\n  env: test\ninstall:\n  status: installed\nauth:\n  jwt_secret: \"\"\nserver:\n  secret_key: \"\"\n"), 0600))
	for _, command := range []string{"migrate", "status"} {
		_, err := loadDatabaseCommandConfig(command, path)
		require.NoError(t, err)
	}
	for _, command := range []string{"seed", "refresh", "backup-run", "prepare-plugin-runtime-credentials"} {
		_, err := loadDatabaseCommandConfig(command, path)
		require.Error(t, err, command)
	}
}
