package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/instanceidentity"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRedisScopeUsesOnlyExistingEnvironmentID(t *testing.T) {
	cfg := GetDefaults()
	_, err := cfg.RedisNamespace()
	require.Error(t, err)
	cfg.EnvironmentID = "22164c23-28b9-440d-9377-5792a438d0e5"
	a, err := cfg.RedisNamespace()
	require.NoError(t, err)
	require.Equal(t, cfg.EnvironmentID, a)
	cfg.Deployment.Env = "prod"
	cfg.Queue.Redis.DB, cfg.Cache.DB = 6, 7
	cfg.Database.DSN = "postgres://other:secret@different:5432/renamed?sslmode=disable"
	b, err := cfg.RedisNamespace()
	require.NoError(t, err)
	require.Equal(t, a, b)
	for _, id := range []string{"", " environment", "env:other", "*", "{env}", "env\n"} {
		cfg.EnvironmentID = id
		_, err = cfg.RedisNamespace()
		require.Error(t, err)
	}
}

func TestEnvironmentIdentityReadsExistingRecordAndDoesNotCreateAnother(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "config.yaml")
	cfg := GetDefaults()
	cfg.SystemSnapshots.Directory = filepath.Join(root, "snapshots")
	require.NoError(t, cfg.ReadEnvironmentIdentity(file))
	require.Empty(t, cfg.EnvironmentID)
	identityFile := cfg.EnvironmentIdentityFile()
	require.NoFileExists(t, identityFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(identityFile), 0700))
	record := map[string]string{"protocol": instanceidentity.CurrentProtocol, "id": "existing-environment", "config_file": file}
	raw, err := json.Marshal(record)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(identityFile, raw, 0600))
	require.NoError(t, cfg.ReadEnvironmentIdentity(file))
	require.Equal(t, record["id"], cfg.EnvironmentID)
	after, err := os.ReadFile(identityFile)
	require.NoError(t, err)
	require.Equal(t, raw, after)
	record["config_file"] = filepath.Join(root, "other.yaml")
	raw, err = json.Marshal(record)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(identityFile, raw, 0600))
	require.Error(t, cfg.ReadEnvironmentIdentity(file))
}

func TestEnvironmentIdentityRespectsExternalEnvironmentFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "config.yaml")
	// The maintenance CLI does not need to source powerx.env into its process.
	require.NoError(t, os.WriteFile(filepath.Join(root, "powerx.env"), []byte("POWERX_LINKS_ROOT="+root+"\n"), 0600))
	original, present := os.LookupEnv("POWERX_LINKS_ROOT")
	require.NoError(t, os.Unsetenv("POWERX_LINKS_ROOT"))
	t.Cleanup(func() {
		if present {
			_ = os.Setenv("POWERX_LINKS_ROOT", original)
		} else {
			_ = os.Unsetenv("POWERX_LINKS_ROOT")
		}
	})
	cfg := GetDefaults()
	require.NoError(t, cfg.ReadEnvironmentIdentity(file))
	require.Equal(t, filepath.Join(instanceidentity.CurrentDirectory(filepath.Join(root, "storage", "system-snapshots"), file), "installation.json"), cfg.EnvironmentIdentityFile())
}

func TestEnvironmentIdentityIsNotAnotherUserSetting(t *testing.T) {
	cfg := GetDefaults()
	cfg.EnvironmentID = "runtime-existing-environment"
	raw, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NotContains(t, string(raw), cfg.EnvironmentID)
	require.NoError(t, yaml.Unmarshal([]byte("environment_id: forged\n"), cfg))
	require.Equal(t, "runtime-existing-environment", cfg.EnvironmentID)
	file := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte("queue:\n  namespace: obsolete\n"), 0600))
	require.ErrorContains(t, loadFromYAML(cfg, file), "queue.namespace is not supported")
}
