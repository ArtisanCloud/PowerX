package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	dbcfg "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func recoveryConfigFixture(t *testing.T) *Config {
	t.Helper()
	root, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	seal := RecoverySeal{Protocol: RecoveryProtocol, JobID: uuid.NewString(), Root: root, Token: uuid.NewString() + uuid.NewString()}
	raw, e := json.Marshal(seal)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(root, "recovery-seal.json"), raw, 0600))
	hash := sha256.Sum256(raw)
	t.Setenv("POWERX_RECOVERY_TOKEN", seal.Token)
	c := GetDefaults()
	c.Deployment.Env = "dev"
	c.Recovery = RecoveryConfig{Mode: "verify", Root: root, JobID: seal.JobID, SealSHA256: hex.EncodeToString(hash[:]), HTTPSocket: filepath.Join(root, "core.sock"), PluginPortMin: 31000, PluginPortMax: 31010}
	c.Database.Driver = "postgres"
	c.Database.DSN = "host=" + filepath.Join(root, "s") + " user=powerx_recovery dbname=powerx sslmode=disable"
	c.Storage.DefaultDriver = "local"
	c.Storage.Local.BasePath = filepath.Join(root, "media")
	c.Plugin.InstalledDir = filepath.Join(root, "plugins")
	c.Plugin.RegistryFile = filepath.Join(root, "registry.json")
	c.Cache.Host = "127.0.0.1"
	c.Cache.Port = 16379
	c.Server.Host = "127.0.0.1"
	c.Server.Port = 18077
	c.Server.GRPC.Host = "127.0.0.1"
	c.Queue.Driver = "redis"
	c.Queue.Redis.Addr = "127.0.0.1:16379"
	c.Event.Bus.Type = "redis"
	c.Event.Bus.RedisAddr = c.Queue.Redis.Addr
	c.Event.Fabric.RedisAddr = c.Queue.Redis.Addr
	c.Event.Fabric.Authorization.RedisAddr = c.Queue.Redis.Addr
	c.KnowledgeSpace.RedisAddr = c.Queue.Redis.Addr
	return c
}
func TestRecoveryStartupRejectsProductionTargetsBeforeBootstrap(t *testing.T) {
	for name, change := range map[string]func(*Config){"database": func(c *Config) { c.Database.DSN = "postgres://source.example.invalid/production" }, "media": func(c *Config) { c.Storage.Local.BasePath = "/production/media" }, "registry": func(c *Config) { c.Plugin.RegistryFile = "/production/registry.json" }, "queue": func(c *Config) { c.Queue.Redis.Addr = "production.example.invalid:6379" }, "events": func(c *Config) { c.Event.Bus.RedisAddr = "production.example.invalid:6379" }, "replica": func(c *Config) {
		c.Database.Replicas = []dbcfg.ReplicaConfig{{Host: "source.example.invalid", Database: "production"}}
	}, "mode": func(c *Config) { c.Recovery.Mode = "unrecognized" }} {
		t.Run(name, func(t *testing.T) {
			cfg := recoveryConfigFixture(t)
			require.NoError(t, cfg.ValidateRecoveryStartup())
			change(cfg)
			require.Error(t, cfg.ValidateRecoveryStartup())
		})
	}
}

func TestVerificationServerDisablesBackgroundWritesAfterNormalStartup(t *testing.T) {
	t.Setenv("POWERX_LOCAL_REHEARSAL", "")
	t.Setenv("POWERX_COORDINATOR_MAINTENANCE", "")
	t.Setenv("POWERX_RECOVERY_VERIFY_SERVER", "1")
	cfg := GetDefaults()
	cfg.Deployment.Env = "prod"
	cfg.Recovery.Mode = "active"
	require.True(t, BackgroundWorkersDisabled(cfg))
}
func TestRecoveryConfigurationDoesNotLoadSourceEnvironment(t *testing.T) {
	cfg := recoveryConfigFixture(t)
	cfg.SystemSnapshots.Directory = filepath.Join(cfg.Recovery.Root, "system-snapshots")
	raw, e := yaml.Marshal(cfg)
	require.NoError(t, e)
	file := filepath.Join(cfg.Recovery.Root, "target.yaml")
	require.NoError(t, os.WriteFile(file, raw, 0600))
	t.Setenv("CORE_X_DB_DSN", "postgres://source.example.invalid/production")
	t.Setenv("POWERX_SNAPSHOT_DIRECTORY", filepath.Join(t.TempDir(), "production snapshots"))
	t.Setenv("POWERX_BACKEND_PORT", "8077")
	previous, path := GlobalConfig, globalConfigPath
	t.Cleanup(func() { GlobalConfig = previous; globalConfigPath = path })
	require.NoError(t, InitGlobalConfig(file))
	require.Equal(t, cfg.Database.DSN, GlobalConfig.Database.DSN)
	require.Equal(t, cfg.SystemSnapshots.Directory, GlobalConfig.SystemSnapshots.Directory)
	require.Equal(t, 18077, GlobalConfig.Server.Port)
	require.True(t, BackgroundWorkersDisabled(GlobalConfig))
}
func TestRecoverySealAndModes(t *testing.T) {
	cfg := recoveryConfigFixture(t)
	require.NoError(t, cfg.ValidateRecoveryStartup())
	cfg.Recovery.Mode = "active"
	require.NoError(t, cfg.ValidateRecoveryStartup())
	require.False(t, BackgroundWorkersDisabled(cfg))
	t.Setenv("POWERX_RECOVERY_TOKEN", "wrong")
	require.Error(t, cfg.ValidateRecoveryStartup())
}

func TestRecoveryRetainsInProcessLocalEventBus(t *testing.T) {
	cfg := recoveryConfigFixture(t)
	cfg.Event.Bus.Type = "local"
	require.NoError(t, cfg.ValidateRecoveryStartup())
	cfg.Event.Bus.Type = "unregistered-external"
	require.Error(t, cfg.ValidateRecoveryStartup())
}
