package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	storageobserver "github.com/ArtisanCloud/PowerX/internal/infra/storage_observer"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"gorm.io/gorm"
)

func TestStorageBootstrapBindsShellAndConfiguredAliasesOnce(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actual, alias, other := filepath.Join(root, "installation"), filepath.Join(root, "current"), filepath.Join(root, "other")
	for _, dir := range []string{"logs/debug", "logs/audit", "media", "snapshots", "backups"} {
		if err = os.MkdirAll(filepath.Join(actual, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"info.log", "error.log"} {
		if err = os.WriteFile(filepath.Join(actual, "logs", name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.Symlink(actual, alias); err != nil {
		t.Fatal(err)
	}
	t.Chdir(actual)
	t.Setenv("PWD", alias) // 模拟 shell 从 macOS /var 别名启动，而内核 cwd 已解析为 /private/var。
	if cwd, e := os.Getwd(); e != nil || cwd != alias {
		t.Fatalf("alias fixture not active: %q %v", cwd, e)
	}
	cfg := &config.Config{EnvironmentID: "alias-install", Deployment: config.DeploymentConfig{Env: "dev"}, StorageGovernance: config.StorageGovernanceConfig{FilesystemPath: actual}}
	cfg.LogConfig.File.Enable = true
	cfg.LogConfig.File.InfoFilePath, cfg.LogConfig.File.ErrorFilePath = "logs/info.log", "logs/error.log"
	cfg.LogConfig.AgentDebug.Enable, cfg.LogConfig.AgentDebug.Dir = true, "logs/debug"
	cfg.Audit.File.Enable, cfg.Audit.File.Dir = true, "logs/audit"
	cfg.Storage.Local.BasePath = filepath.Join(alias, "media")
	cfg.SystemSnapshots.Directory = filepath.Join(alias, "snapshots")
	deps := &shared.Deps{}
	if err = configureStorageGovernance(cfg, deps, archiveops.Config{}, nil, filepath.Join(alias, "backups")); err != nil {
		t.Fatal(err)
	}
	out, err := deps.StorageGovernance.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out.Resources {
		if r.State != "observed" || !strings.HasPrefix(r.Path, actual+string(os.PathSeparator)) {
			t.Fatalf("configured alias was not bound to actual installation: %+v", r)
		}
	}
	if err = os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(other, alias); err != nil {
		t.Fatal(err)
	}
	resources := make([]storagegov.Resource, 0, len(out.Resources))
	for _, r := range out.Resources {
		resources = append(resources, r.Resource)
	}
	again := (storageobserver.Collector{}).Collect(context.Background(), actual, resources)
	for _, r := range again.Resources {
		if r.State != "observed" {
			t.Fatalf("registration followed retargeted alias: %+v", r)
		}
	}
	missing := filepath.Join(alias, "not-created", "child")
	if got := bindStorageSourcePath(missing); got != filepath.Join(other, "not-created", "child") {
		t.Fatal(got)
	}
	if _, err = os.Stat(filepath.Join(other, "not-created")); !os.IsNotExist(err) {
		t.Fatal("registration created a missing directory")
	}
}

func TestStorageBootstrapUsesInstallationIdentityAndDoesNotCreateSources(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{EnvironmentID: "actual-installation", Deployment: config.DeploymentConfig{Env: "prod"}, StorageGovernance: config.StorageGovernanceConfig{FilesystemPath: root}}
	cfg.SystemSnapshots.Directory = filepath.Join(root, "snapshot-source")
	cfg.StorageGovernance.Sources = []config.StorageObserverSource{{ID: "shared-journal", Name: "Journal", Kind: "journal", Scope: "shared", Path: filepath.Join(root, "journal-source")}}
	deps := &shared.Deps{}
	if err := configureStorageGovernance(cfg, deps, archiveops.Config{Directory: filepath.Join(root, "archives")}, nil, filepath.Join(root, "backups")); err != nil {
		t.Fatal(err)
	}
	out, err := deps.StorageGovernance.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Identity.InstanceID != "actual-installation" || out.Identity.Environment != "prod" || out.CleanupEnabled || len(out.Resources) != 4 {
		t.Fatalf("wrong identity or sources: %+v", out)
	}
	for _, r := range out.Resources {
		if r.ID != "shared-journal" && (r.Scope != "prod" || r.InstanceID != "actual-installation") {
			t.Fatal("scope guessed from source path")
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("observation created source directories: %v %v", entries, err)
	}
}

func TestStorageBootstrapDiscoversApplicationLogsAndMediaWithoutScanningOtherFiles(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"info.log", "error.log", "unrelated-private-file"} {
		if err = os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{EnvironmentID: "actual-install", Deployment: config.DeploymentConfig{Env: "dev"}, StorageGovernance: config.StorageGovernanceConfig{FilesystemPath: root}}
	cfg.LogConfig.File.Enable = true
	cfg.LogConfig.File.InfoFilePath = filepath.Join(root, "info.log")
	cfg.LogConfig.File.ErrorFilePath = filepath.Join(root, "error.log")
	cfg.LogConfig.File.MaxAge = 7
	cfg.Storage.Local.BasePath = filepath.Join(root, "media")
	cfg.SystemSnapshots.Directory = filepath.Join(root, "snapshots")
	deps := &shared.Deps{}
	if err = configureStorageGovernance(cfg, deps, archiveops.Config{}, nil, filepath.Join(root, "backups")); err != nil {
		t.Fatal(err)
	}
	out, err := deps.StorageGovernance.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range out.Resources {
		if r.Kind == "application_logs" {
			found = true
			if r.Files != 2 || r.Scope != "dev" || r.InstanceID != "actual-install" {
				t.Fatalf("wrong log attribution: %+v", r)
			}
		}
	}
	if !found {
		t.Fatal("configured logs omitted")
	}
	states := map[string]string{}
	for _, c := range out.Inventory {
		states[c.Kind] = c.State
	}
	if states["application_logs"] != "connected" || states["media_files"] != "connected" || states["temporary_files"] != "not_connected" {
		t.Fatal(states)
	}
	if runtime.GOOS != "linux" && states["journal"] != "not_applicable" {
		t.Fatal(states)
	}
	if _, err = os.Stat(cfg.Storage.Local.BasePath); !os.IsNotExist(err) {
		t.Fatal("observer created media directory")
	}
}

func TestStorageBootstrapSamplingReportsMaintenanceAdmission(t *testing.T) {
	t.Setenv("POWERX_COORDINATOR_MAINTENANCE", "1")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{EnvironmentID: "existing-instance", Deployment: config.DeploymentConfig{Env: "dev"}, StorageGovernance: config.StorageGovernanceConfig{FilesystemPath: root, SamplingEnabled: true}}
	cfg.SystemSnapshots.Directory = root
	deps := &shared.Deps{DB: &gorm.DB{}}
	if err = configureStorageGovernance(cfg, deps, archiveops.Config{}, nil, root); err != nil {
		t.Fatal(err)
	}
	out, err := deps.StorageGovernance.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !out.Sampling.Configured || out.Sampling.WorkerEnabled || out.Sampling.IntervalSeconds != 300 || out.CleanupEnabled {
		t.Fatalf("maintenance became running/safe: %+v", out.Sampling)
	}
}

func TestStorageBootstrapDoesNotEnableUnsupportedModes(t *testing.T) {
	deps := &shared.Deps{}
	err := configureStorageGovernance(&config.Config{StorageGovernance: config.StorageGovernanceConfig{Mode: "enforce"}}, deps, archiveops.Config{}, nil, "/backups")
	if err == nil || deps.StorageGovernance != nil {
		t.Fatal("unsupported mode silently enabled")
	}
}
