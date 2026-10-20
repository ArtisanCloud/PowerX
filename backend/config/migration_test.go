package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationConfigurationDoesNotRequireRuntimeSecrets(t *testing.T) {
	isolateInstanceEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	for _, secret := range []string{"\"\"", legacyDefaultJWTSecret, "short"} {
		body := []byte("deployment:\n  env: test\ninstall:\n  status: installed\nauth:\n  jwt_secret: " + secret + "\n  access_ttl: invalid\nserver:\n  secret_key: invalid\n  port: -1\n")
		require.NoError(t, os.WriteFile(path, body, 0600))
		_, err := LoadForMigration(path)
		require.NoError(t, err)
		_, err = Load(path)
		require.Error(t, err, "服务配置校验必须保持严格")
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, body, after, "迁移不能生成或修改任何密钥")
	}
	t.Setenv("CORE_X_AUTH_JWT_SECRET", "deprecated-unused-for-migration")
	t.Setenv("POWERX_AUTH_JWT_SECRET", "short")
	_, err := LoadForMigration(path)
	require.NoError(t, err)
	_, err = Load(path)
	require.ErrorContains(t, err, "已不再支持")
}

func TestMigrationConfigurationStillValidatesDatabaseAndDeployment(t *testing.T) {
	isolateInstanceEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	_, err := LoadForMigration(path)
	require.ErrorContains(t, err, "存在的 config.yaml")
	for _, body := range []string{
		"deployment:\n  env: invalid\n",
		"deployment:\n  env: test\ndatabase:\n  port: -1\n",
		"deployment:\n  env: test\ndatabase:\n  driver: unsupported\n",
		"deployment:\n  env: test\ndatabase:\n  max_open_conns: 0\n",
	} {
		require.NoError(t, os.WriteFile(path, []byte(body), 0600))
		_, err := LoadForMigration(path)
		require.Error(t, err)
	}
	key := randomTestJWTSecret(t)
	path = writeInstanceEnvironmentFixture(t, t.TempDir(), "test", key)
	cfg, err := LoadForMigration(path)
	require.NoError(t, err)
	require.Equal(t, "test_database", cfg.Database.Database)
	require.Equal(t, "literal$PASSWORD", cfg.Database.Password)
	t.Setenv("POWERX_UPGRADE_MODE", "1")
	_, err = LoadForMigration(path)
	require.Error(t, err, "升级模式不能通过迁移入口放松校验")
}
