package system_recovery

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCurrentCapabilityCatalogLegacyCompatibility(t *testing.T) {
	for _, scenario := range []struct {
		name, core, failure string
		catalog, current    bool
	}{
		{name: "legacy_missing_catalog_preserves_adapter_catalog", core: "old-core", current: true},
		{name: "legacy_with_catalog_restores_it", core: "old-core", catalog: true, current: true},
		{name: "modern_missing_catalog_fails", core: CurrentRecoveryProtocol + CurrentCacheProtocol, current: true, failure: "快照正式能力目录不可读取"},
		{name: "legacy_cannot_run_without_current_catalog", core: "old-core", failure: "当前 Core 配套目录也不可用"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			r, cfg, source := currentFixture(t)
			root, err := filepath.EvalSymlinks(filepath.Dir(r.spec.ConfigFile))
			require.NoError(t, err)
			r.spec.ConfigFile, r.spec.Executable = filepath.Join(root, "config.yaml"), filepath.Join(root, "powerx")
			cfg.Plugin.InstalledDir = ""
			cfg.Auth.JWTSecret = token()
			cfg.Server.SecretKey = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("r", 32)))
			t.Setenv("POWERX_AUTH_JWT_SECRET", cfg.Auth.JWTSecret)
			t.Setenv("CORE_X_AUTH_ISSUER", cfg.Auth.Issuer)
			source.MediaRoot = filepath.Join(root, "media")
			catalog := filepath.Join(root, "capabilities")
			source.Artifacts["core/platform_capabilities"] = catalog
			if scenario.current {
				require.NoError(t, os.MkdirAll(catalog, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(catalog, "contract.yaml"), []byte("current"), 0600))
			}
			staging, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Join(staging, "programs/core"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(staging, "programs/core/powerx"), []byte(scenario.core), 0700))
			if scenario.catalog {
				dir := filepath.Join(staging, "programs/core/platform_capabilities")
				require.NoError(t, os.MkdirAll(dir, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "contract.yaml"), []byte("historical"), 0600))
			}
			job := snapshots.ManagedJob{ID: uuid.NewString()}
			require.NoError(t, privateDirectory(r.work(job.ID)))
			journal, err := r.prepareCurrentFiles(context.Background(), &job, source, staging, nil, cfg)
			if scenario.failure != "" {
				require.ErrorContains(t, err, scenario.failure)
				require.Empty(t, journal.Files, "缺少目录时必须在准备与覆盖之前拒绝")
				return
			}
			require.NoError(t, err)
			require.NoError(t, r.installCurrentFiles(context.Background(), &journal))
			data, err := os.ReadFile(filepath.Join(catalog, "contract.yaml"))
			require.NoError(t, err)
			if scenario.catalog {
				require.Equal(t, "historical", string(data))
				require.NotContains(t, checkNames(job.Checks), "local_capability_adapter")
			} else {
				require.Equal(t, "current", string(data))
				require.Contains(t, checkNames(job.Checks), "local_capability_adapter")
			}
			require.NoError(t, r.revertCurrentFiles(journal))
		})
	}
}

func checkNames(checks []snapshots.RecoveryCheck) []string {
	names := make([]string, 0, len(checks))
	for _, check := range checks {
		names = append(names, check.Name)
	}
	return names
}

// 显式读取真实旧包，但只在测试拥有的目录内准备文件，不连接业务数据库或替换现场。
func TestCurrentRegisteredSnapshotPreparationAudit(t *testing.T) {
	directory, id := os.Getenv("POWERX_RECOVERY_PATH_AUDIT_DIRECTORY"), os.Getenv("POWERX_RECOVERY_SNAPSHOT_AUDIT_ID")
	if directory == "" || id == "" {
		t.Skip("explicit read-only snapshot audit with owned preparation targets")
	}
	service := snapshots.NewService(context.Background(), snapshots.Config{Directory: directory}, snapshots.Options{Coordinator: true})
	defer service.Close()
	registered, err := service.LoadCoordinatorSource()
	require.NoError(t, err)
	staging := filepath.Join(t.TempDir(), "materialized")
	manifest, configuration, err := service.MaterializeForRecovery(privileged(context.Background()), id, staging)
	require.NoError(t, err)
	r, cfg, source := currentFixture(t)
	root, err := filepath.EvalSymlinks(filepath.Dir(r.spec.ConfigFile))
	require.NoError(t, err)
	r.spec.ConfigFile, r.spec.Executable = filepath.Join(root, "config.yaml"), filepath.Join(root, "powerx")
	cfg.Plugin.InstalledDir, cfg.Plugin.RegistryFile = filepath.Join(root, "plugins"), filepath.Join(root, "registry.json")
	cfg.Auth.JWTSecret = token()
	cfg.Server.SecretKey = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("r", 32)))
	t.Setenv("POWERX_AUTH_JWT_SECRET", cfg.Auth.JWTSecret)
	t.Setenv("CORE_X_AUTH_ISSUER", cfg.Auth.Issuer)
	source.MediaRoot = filepath.Join(root, "media")
	source.Artifacts["core/web-admin"] = filepath.Join(root, "web")
	source.Artifacts["core/platform_capabilities"] = filepath.Join(root, "capabilities")
	require.NoError(t, snapshots.CopyLocalProgram(context.Background(), registered.Artifacts["core/platform_capabilities"], source.Artifacts["core/platform_capabilities"]))
	for logical := range registered.ConfigFiles {
		if strings.HasPrefix(logical, "core/files/") {
			source.ConfigFiles[logical] = filepath.Join(root, "aux", digestPath(logical))
		}
	}
	job := snapshots.ManagedJob{ID: uuid.NewString()}
	require.NoError(t, privateDirectory(r.work(job.ID)))
	journal, err := r.prepareCurrentFiles(context.Background(), &job, source, staging, configuration, cfg)
	require.NoError(t, err)
	require.NotEmpty(t, journal.Files)
	for _, file := range journal.Files {
		require.True(t, inside(root, file.Target), "仅准备测试拥有的目录")
	}
	require.NoError(t, r.revertCurrentFiles(journal))
	t.Logf("verified snapshot %s: %d manifest files, %d owned preparation targets; checks=%v; no live database or file replacement", id, len(manifest.Files), len(journal.Files), checkNames(job.Checks))
}
