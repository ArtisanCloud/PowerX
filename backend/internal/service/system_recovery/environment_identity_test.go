package system_recovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/instanceidentity"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestEnvironmentIdentityReusesCurrentEnvironmentBeforeRedisBootstrap(t *testing.T) {
	r, cfg, _ := currentFixture(t)
	active, err := r.Coordinator.Active()
	require.NoError(t, err)
	require.Equal(t, active.Deployment.ID, cfg.EnvironmentID)
	originalID := cfg.EnvironmentID
	record := filepath.Join(r.spec.Directory, "installation.json")
	before, err := os.ReadFile(record)
	require.NoError(t, err)
	cfg.EnvironmentID = ""
	cfg.Queue.Redis.DB, cfg.Cache.DB = 6, 7
	cfg.Database.DSN = "postgres://rotated@other/current_fixture?sslmode=disable"
	require.NoError(t, EnsureEnvironmentIdentity(cfg, r.spec.ConfigFile))
	require.Equal(t, originalID, cfg.EnvironmentID)
	after, err := os.ReadFile(record)
	require.NoError(t, err)
	require.Equal(t, before, after, "已有环境只读复用，不生成另一套身份")
}

func TestRecoverySourceKeepsLiveEnvironmentIDOutsideSnapshotYAML(t *testing.T) {
	r, cfg, source := currentFixture(t)
	raw, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NotContains(t, string(raw), cfg.EnvironmentID)
	source.EffectiveConfig = raw
	restored, err := r.currentSourceConfig(source)
	require.NoError(t, err)
	id, err := restored.RedisNamespace()
	require.NoError(t, err)
	require.Equal(t, r.spec.ID, id)
}

func TestFirstInstallationInitializesExistingRecordOnce(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "config.yaml")
	cfg := config.GetDefaults()
	cfg.Server.Host = "127.0.0.1"
	cfg.Deployment.Env = "dev"
	cfg.SystemSnapshots.Directory = filepath.Join(root, "snapshots")
	cfg.Storage.DefaultDriver = "local"
	cfg.Storage.Local.BasePath = filepath.Join(root, "media")
	cfg.Database.Driver, cfg.Database.DSN = "postgres", "postgres://fixture@localhost/new_fixture?sslmode=disable"
	require.NoError(t, os.MkdirAll(cfg.Storage.Local.BasePath, 0700))
	require.NoError(t, writeInstallationConfig(file, cfg))
	require.NoError(t, EnsureEnvironmentIdentity(cfg, file))
	originalID := cfg.EnvironmentID
	require.True(t, instanceidentity.Valid(originalID))
	var record currentSpec
	require.NoError(t, snapshots.ReadCoordinatorJSON(cfg.EnvironmentIdentityFile(), &record))
	require.Equal(t, originalID, record.ID)
	require.Equal(t, file, record.ConfigFile)
	require.Equal(t, "127.0.0.1:8077", record.Listen)
	source := snapshots.Config{Directory: cfg.SystemSnapshots.Directory, DatabaseDSN: cfg.Database.DSN, MediaRoot: cfg.Storage.Local.BasePath, ConfigFiles: map[string]string{}, Artifacts: map[string]string{}}
	service := snapshots.NewService(context.Background(), source, snapshots.Options{Coordinator: true})
	t.Cleanup(service.Close)
	runtime, err := NewCurrentRuntime(context.Background(), cfg, file, source, service)
	require.NoError(t, err)
	t.Cleanup(runtime.Close)
	active, err := runtime.Coordinator.Active()
	require.NoError(t, err)
	require.Equal(t, originalID, active.Deployment.ID)
	cfg.EnvironmentID = ""
	require.NoError(t, EnsureEnvironmentIdentity(cfg, file))
	require.Equal(t, originalID, cfg.EnvironmentID, "重启仍读取环境管理的同一记录")
}
