package config

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func installedUpgradeConfig(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	data := filepath.Join(root, "data")
	for _, sub := range []string{"uploads", "plugins/installed", "runtime"} {
		require.NoError(t, os.MkdirAll(filepath.Join(data, sub), 0700))
	}
	cfg := GetDefaults()
	cfg.Install.Status = "installed"
	cfg.Deployment.Env = "dev"
	cfg.Database.Driver = "postgres"
	cfg.Database.Host = "postgres"
	cfg.Database.Database = "existing"
	cfg.Database.UserName = "existing"
	cfg.Database.Password = "preserve-existing-password"
	secret := make([]byte, 48)
	_, err := rand.Read(secret)
	require.NoError(t, err)
	cfg.Auth.JWTSecret = base64.RawURLEncoding.EncodeToString(secret)
	_, err = rand.Read(secret)
	require.NoError(t, err)
	cfg.Server.SecretKey = base64.StdEncoding.EncodeToString(secret[:32])
	cfg.Storage.DefaultDriver = "local"
	cfg.Storage.Local.BasePath = filepath.Join(data, "uploads")
	body, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	path := filepath.Join(root, "config.yaml")
	require.NoError(t, os.WriteFile(path, body, 0600))
	return path, data
}

func TestUpgradeConfigPreservesInstalledConfigAndExplicitEnvironment(t *testing.T) {
	t.Setenv("POWERX_ENV", "dev")
	path, data := installedUpgradeConfig(t)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	first, hash, err := LoadForUpgrade(path, data)
	require.NoError(t, err)
	require.Equal(t, "installed", first.Install.Status)
	require.Equal(t, "preserve-existing-password", first.Database.Password)
	second, next, err := LoadForUpgrade(path, data)
	require.NoError(t, err)
	require.Equal(t, hash, next)
	require.Equal(t, first.Auth.JWTSecret, second.Auth.JWTSecret)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	t.Setenv("POWERX_AUTH_JWT_SECRET", randomTestJWTSecret(t))
	overridden, changed, err := LoadForUpgrade(path, data)
	require.NoError(t, err)
	require.NotEqual(t, hash, changed)
	require.NotEqual(t, overridden.Auth.JWTSecret, first.Auth.JWTSecret)
	after, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestUpgradeConfigRejectsMissingCorruptUninstalledAndMounts(t *testing.T) {
	t.Setenv("POWERX_ENV", "dev")
	for _, variant := range []string{"missing", "empty", "yaml", "uninstalled", "permissions", "mount", "binding"} {
		t.Run(variant, func(t *testing.T) {
			path, data := installedUpgradeConfig(t)
			switch variant {
			case "missing":
				require.NoError(t, os.Remove(path))
			case "empty":
				require.NoError(t, os.WriteFile(path, []byte(""), 0600))
			case "yaml":
				require.NoError(t, os.WriteFile(path, []byte("password: [SECRET_INVALID_YAML"), 0600))
			case "permissions":
				require.NoError(t, os.Chmod(path, 0644))
			case "mount":
				require.NoError(t, os.Remove(filepath.Join(data, "runtime")))
			case "uninstalled", "binding":
				body, err := os.ReadFile(path)
				require.NoError(t, err)
				var cfg Config
				require.NoError(t, yaml.Unmarshal(body, &cfg))
				if variant == "uninstalled" {
					cfg.Install.Status = "uninstalled"
				} else {
					cfg.Database.Password = ""
				}
				body, err = yaml.Marshal(cfg)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, body, 0600))
			}
			before, _ := os.ReadFile(path)
			_, _, err := LoadForUpgrade(path, data)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "SECRET_INVALID_YAML")
			after, _ := os.ReadFile(path)
			require.Equal(t, before, after)
		})
	}
}

func TestUpgradeModeBlocksDefaultAndGlobalConfigFallback(t *testing.T) {
	t.Setenv("POWERX_ENV", "dev")
	t.Setenv("POWERX_UPGRADE_MODE", "1")
	path, data := installedUpgradeConfig(t)
	t.Setenv("POWERX_UPGRADE_DATA_DIR", data)
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.ErrorContains(t, err, "UPGRADE_CONFIG_UNREADABLE")
	previous, previousPath := GlobalConfig, globalConfigPath
	t.Cleanup(func() { GlobalConfig = previous; globalConfigPath = previousPath })
	require.ErrorContains(t, InitGlobalConfig(filepath.Join(t.TempDir(), "missing.yaml")), "UPGRADE_CONFIG_UNREADABLE")
	require.NoError(t, InitGlobalConfig(path))
	require.Equal(t, "installed", GlobalConfig.Install.Status)
}
