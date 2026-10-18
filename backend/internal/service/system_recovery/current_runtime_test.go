package system_recovery

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func currentFixture(t *testing.T) (*CurrentRuntime, *config.Config, snapshots.Config) {
	t.Helper()
	root := t.TempDir()
	cfg := config.GetDefaults()
	cfg.Server.Host = "127.0.0.1"
	cfg.Deployment.Env = "dev"
	cfg.SystemSnapshots.Directory = filepath.Join(root, "snapshots")
	cfg.Recovery = config.RecoveryConfig{}
	cfg.Storage.DefaultDriver = "local"
	cfg.Storage.Local.BasePath = filepath.Join(root, "media")
	cfg.Database.Driver = "postgres"
	cfg.Database.DSN = "postgres://fixture@localhost/current_fixture?sslmode=disable"
	require.NoError(t, os.MkdirAll(cfg.Storage.Local.BasePath, 0700))
	raw, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	source := snapshots.Config{Directory: cfg.SystemSnapshots.Directory, DatabaseDSN: cfg.Database.DSN, MediaRoot: cfg.Storage.Local.BasePath, EffectiveConfig: raw, ConfigFiles: map[string]string{}, Artifacts: map[string]string{}}
	service := snapshots.NewService(context.Background(), source, snapshots.Options{Coordinator: true})
	t.Cleanup(service.Close)
	file := filepath.Join(root, "config.yaml")
	require.NoError(t, writeInstallationConfig(file, cfg))
	r, err := NewCurrentRuntime(context.Background(), cfg, file, source, service)
	require.NoError(t, err)
	require.NotNil(t, r)
	t.Cleanup(r.Close)
	return r, cfg, source
}

func TestCurrentRuntimeNormalStartupOwnsNoNewProcessAndUsesOriginalPort(t *testing.T) {
	r, cfg, _ := currentFixture(t)
	require.Equal(t, "127.0.0.1:8077", r.spec.Listen)
	require.Equal(t, recoveryEntry, r.Coordinator.Store.RecoveryEntry)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{}, 1)
	require.NoError(t, r.StartNormal(ctx, func() { stopped <- struct{}{} }))
	require.True(t, r.Coordinator.Store.Online())
	_, err := r.Coordinator.Store.EnqueueInstanceRestore(privileged(ctx), uuid.NewString(), uuid.NewString(), 1)
	require.NoError(t, err)
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("恢复账本事件没有触发维护重启")
	}
	require.True(t, r.TransitionRequested())
	startup, err := CurrentStartup(cfg, r.spec.ConfigFile)
	require.NoError(t, err)
	require.NotNil(t, startup)
	defer startup.Coordinator.SnapshotStore.Close()
	require.False(t, startup.AwaitingStartup())
}

func TestCurrentStartupOnlyCompletesMatchingNormalBoot(t *testing.T) {
	r, _, _ := currentFixture(t)
	job, err := r.Coordinator.Store.EnqueueInstanceRestore(privileged(context.Background()), uuid.NewString(), uuid.NewString(), 1)
	require.NoError(t, err)
	job.Status, job.Phase = "running", "verifying_startup"
	require.NoError(t, r.Coordinator.Store.Save(job))
	require.False(t, r.AwaitingStartup())
	t.Setenv("POWERX_RECOVERY_BOOT_JOB", uuid.NewString())
	require.False(t, r.AwaitingStartup())
	t.Setenv("POWERX_RECOVERY_BOOT_JOB", job.ID)
	require.True(t, r.AwaitingStartup())
	previous := config.GlobalConfig
	config.GlobalConfig = cfgFromCurrentFixture(t, r.spec.ConfigFile)
	t.Cleanup(func() { config.GlobalConfig = previous })
	require.NoError(t, r.FinishStartup())
	got, err := r.Coordinator.Store.Get(job.ID)
	require.NoError(t, err)
	require.Equal(t, "complete", got.Status)
	active, err := r.Coordinator.Active()
	require.NoError(t, err)
	require.Equal(t, uint64(2), active.Generation)
	require.False(t, active.Maintenance)
	require.NoError(t, r.FinishStartup())
	active, err = r.Coordinator.Active()
	require.NoError(t, err)
	require.Equal(t, uint64(2), active.Generation)
}

func cfgFromCurrentFixture(t *testing.T, file string) *config.Config {
	t.Helper()
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	return &cfg
}

func TestCurrentBeforeConfigFindsInterruptedConfigurationReplacement(t *testing.T) {
	r, _, _ := currentFixture(t)
	_, err := r.Coordinator.Store.EnqueueInstanceRestore(privileged(context.Background()), uuid.NewString(), uuid.NewString(), 1)
	require.NoError(t, err)
	t.Setenv("POWERX_CONFIG", r.spec.ConfigFile)
	require.NoError(t, os.Rename(r.spec.ConfigFile, r.spec.ConfigFile+".before"))
	startup, err := CurrentBeforeConfig()
	require.NoError(t, err)
	require.NotNil(t, startup)
	defer startup.Coordinator.SnapshotStore.Close()
	require.False(t, startup.AwaitingStartup())
}

func TestCurrentBeforeConfigDoesNotBlockNormalBootForMissingBackupSource(t *testing.T) {
	r, _, _ := currentFixture(t)
	t.Setenv("POWERX_CONFIG", r.spec.ConfigFile)
	require.NoError(t, os.Remove(filepath.Join(r.spec.SnapshotDirectory, "source.enc")))
	startup, err := CurrentBeforeConfig()
	require.NoError(t, err)
	require.Nil(t, startup)
}

func TestCurrentFileJournalRevertsInterruptedRenameWithoutDatabase(t *testing.T) {
	r, _, _ := currentFixture(t)
	id := uuid.NewString()
	target := filepath.Join(t.TempDir(), "media")
	backup := target + ".before"
	prepared := target + ".new"
	require.NoError(t, os.MkdirAll(backup, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(backup, "state"), []byte("B"), 0600))
	require.NoError(t, os.MkdirAll(target, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(target, "state"), []byte("partial-A"), 0600))
	journal := currentJournal{JobID: id, Files: []currentFile{{Target: target, Backup: backup, Prepared: prepared, Existed: true, Started: true}}}
	require.NoError(t, privateDirectory(r.work(id)))
	require.NoError(t, snapshots.WriteCoordinatorJSON(r.journalFile(id), journal))
	require.NoError(t, r.compensateCurrent(context.Background(), &snapshots.ManagedJob{ID: id}))
	data, err := os.ReadFile(filepath.Join(target, "state"))
	require.NoError(t, err)
	require.Equal(t, "B", string(data))
	// 补偿后的第二次启动保持原文件，不会把当前状态再次删除。
	require.NoError(t, r.compensateCurrent(context.Background(), &snapshots.ManagedJob{ID: id}))
	data, err = os.ReadFile(filepath.Join(target, "state"))
	require.NoError(t, err)
	require.Equal(t, "B", string(data))
}

func TestCurrentPreparationFailureRemovesOnlyItsTemporaryFiles(t *testing.T) {
	r, cfg, source := currentFixture(t)
	root := filepath.Dir(r.spec.ConfigFile)
	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	r.spec.ConfigFile = filepath.Join(root, "config.yaml")
	cfg.Auth.JWTSecret = token()
	t.Setenv("POWERX_AUTH_JWT_SECRET", cfg.Auth.JWTSecret)
	t.Setenv("CORE_X_AUTH_ISSUER", cfg.Auth.Issuer)
	cfg.Server.SecretKey = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("r", 32)))
	cfg.Plugin.InstalledDir = ""
	r.spec.Executable = filepath.Join(root, "z-core")
	source.MediaRoot = filepath.Join(root, "a-media")
	source.Artifacts["core/web-admin"] = filepath.Join(root, "z-web")
	staging := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(staging, "programs/core"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(staging, "programs/core/powerx"), []byte(CurrentRecoveryProtocol), 0700))
	id := uuid.NewString()
	require.NoError(t, privateDirectory(r.work(id)))
	before, err := os.ReadFile(r.spec.ConfigFile)
	require.NoError(t, err)
	journal, err := r.prepareCurrentFiles(context.Background(), &snapshots.ManagedJob{ID: id}, source, staging, nil, cfg)
	require.Error(t, err, "缺失的 Web 产物必须阻止覆盖")
	require.ErrorContains(t, err, filepath.Join(staging, "programs/core/web-admin"))
	require.ErrorContains(t, err, source.Artifacts["core/web-admin"])
	require.NotEmpty(t, journal.Files, "失败发生在一部分文件准备完成之后: %v", err)
	leftovers, err := filepath.Glob(filepath.Join(root, ".powerx-new-"+id+"-*"))
	require.NoError(t, err)
	require.Empty(t, leftovers)
	after, err := os.ReadFile(r.spec.ConfigFile)
	require.NoError(t, err)
	require.Equal(t, before, after, "预检查失败不改变当前配置")
}

func TestCurrentRuntimeRejectsUnknownConnectionAndSymlinkState(t *testing.T) {
	r, cfg, source := currentFixture(t)
	source.DatabaseDSN = "postgres://fixture@localhost/another_database"
	require.Error(t, r.checkConnection(source))
	file := filepath.Join(t.TempDir(), "state-link")
	require.NoError(t, os.Symlink(r.spec.Directory, file))
	require.Error(t, privateDirectory(file))
	cfg.Recovery.ProfileFile = "/existing/managed-profile.json"
	got, err := NewCurrentRuntime(context.Background(), cfg, r.spec.ConfigFile, source, r.Coordinator.SnapshotStore)
	require.NoError(t, err)
	require.Nil(t, got)
}
