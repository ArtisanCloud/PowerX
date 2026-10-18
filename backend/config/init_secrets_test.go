package config

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestInstallationSecretsArePersistentPrivateAndRetainConfiguration(t *testing.T) {
	isolateInstanceEnvironment(t)
	path := setupSecretTestPath(t, "uninstalled", "\"\"")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString("# 运维自定义注释\ncustom_nested:\n  value: retained\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Chmod(path, 0644))
	result, err := PrepareInstallationSecrets(path)
	require.NoError(t, err)
	require.Equal(t, SecretInitialization{JWT: "generated", Server: "generated"}, result)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(before), "# 运维自定义注释")
	require.Contains(t, string(before), "value: retained")
	var cfg Config
	require.NoError(t, yaml.Unmarshal(before, &cfg))
	require.NoError(t, validateJWTSecret(cfg.Auth.JWTSecret))
	key, err := base64.StdEncoding.DecodeString(cfg.Server.SecretKey)
	require.NoError(t, err)
	require.Len(t, key, 32)
	require.NotEqual(t, cfg.Auth.JWTSecret, cfg.Server.SecretKey)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	result, err = PrepareInstallationSecrets(path)
	require.NoError(t, err)
	require.Equal(t, SecretInitialization{JWT: "existing", Server: "existing"}, result)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, PrepareSetupJWTSecret(path))
	after, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "Setup 沿用手动初始化结果")
}

func TestInstallationSecretsRejectInstalledInvalidAndEnvironmentErrorsWithoutWrites(t *testing.T) {
	isolateInstanceEnvironment(t)
	for _, state := range []string{"installed", "uninstalled"} {
		path := setupSecretTestPath(t, state, "\"\"")
		if state == "uninstalled" {
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(path), "powerx.env"), []byte("POWERX_AUTH_JWT_SECRET=too-short\n"), 0600))
		}
		before, err := os.ReadFile(path)
		require.NoError(t, err)
		_, err = PrepareInstallationSecrets(path)
		require.Error(t, err)
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
	path := setupSecretTestPath(t, "uninstalled", "\"\"")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString("server:\n  secret_key: invalid\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = PrepareInstallationSecrets(path)
	require.ErrorContains(t, err, "server.secret_key")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestInstallationSecretsHonorInstanceAndLocalEnvironment(t *testing.T) {
	isolateInstanceEnvironment(t)
	for _, local := range []bool{false, true} {
		root := t.TempDir()
		path := filepath.Join(root, "etc", "config.yaml")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte("install:\n  status: uninstalled\nauth:\n  jwt_secret: \"\"\n"), 0600))
		key := randomTestJWTSecret(t)
		envPath := filepath.Join(filepath.Dir(path), "powerx.env")
		if local {
			t.Setenv("POWERX_ENV", "dev")
			t.Setenv("POWERX_AUTH_JWT_SECRET", "")
			require.NoError(t, os.Unsetenv("POWERX_AUTH_JWT_SECRET"))
			envPath = filepath.Join(root, ".env")
		}
		require.NoError(t, os.WriteFile(envPath, []byte("POWERX_AUTH_JWT_SECRET="+key+"\n"), 0600))
		result, err := PrepareInstallationSecrets(path)
		require.NoError(t, err)
		require.Equal(t, "environment", result.JWT)
		require.Empty(t, readSetupSecret(t, path), "不复制或覆盖外部管理的 JWT")
		cfg, err := LoadForMigration(path)
		require.NoError(t, err)
		require.NotNil(t, cfg)
	}
}

func TestInstallationSecretsProcessHelper(t *testing.T) {
	if os.Getenv("POWERX_INIT_SECRETS_TEST_CHILD") != "1" {
		t.Skip("仅供并发子进程验证")
	}
	_, err := PrepareInstallationSecrets(os.Getenv("POWERX_CONFIG"))
	require.NoError(t, err)
}

func TestInstallationSecretsRejectMalformedOrSymlinkedConfigWithoutWrites(t *testing.T) {
	isolateInstanceEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	for _, body := range []string{"", "not-a-mapping", "auth:\n  jwt_secret: first\n  jwt_secret: second\n", "auth: [invalid]"} {
		require.NoError(t, os.WriteFile(path, []byte(body), 0600))
		_, err := PrepareInstallationSecrets(path)
		require.Error(t, err)
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, body, string(after))
	}
	link := filepath.Join(filepath.Dir(path), "linked.yaml")
	require.NoError(t, os.Symlink(path, link))
	_, err := PrepareInstallationSecrets(link)
	require.Error(t, err)
}

func TestInstallationSecretsMultipleProcessesKeepOneIdentity(t *testing.T) {
	isolateInstanceEnvironment(t)
	path := setupSecretTestPath(t, "uninstalled", "\"\"")
	binary, err := os.Executable()
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(binary, "-test.run=^TestInstallationSecretsProcessHelper$")
			cmd.Env = append(os.Environ(), "POWERX_INIT_SECRETS_TEST_CHILD=1", "POWERX_CONFIG="+path)
			raw, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", raw)
		}()
	}
	wg.Wait()
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, PrepareSetupJWTSecret(path))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
