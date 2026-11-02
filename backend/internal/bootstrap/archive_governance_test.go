package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
)

func TestArchiveBootstrapResolvesRelativeLogSourcesBeforeValidation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(root)
	t.Setenv("POWERX_OPS_ARCHIVE_CONFIG", "")
	for _, dir := range []string{"logs/audit", "logs/application", "logs/extra"} {
		require.NoError(t, os.MkdirAll(dir, 0700))
	}
	cfg := &config.Config{EnvironmentID: "owned-archive-install"}
	cfg.Audit.File.Enable, cfg.Audit.File.Dir = true, "logs/audit"
	cfg.LogConfig.File.Enable = true
	cfg.LogConfig.File.InfoFilePath = "logs/application/info.log"
	cfg.LogConfig.File.ErrorFilePath = "logs/application/error.log"
	cfg.StorageGovernance.Sources = []config.StorageObserverSource{{ID: "extra-logs", Name: "Extra logs", Kind: "application_logs", Path: "logs/extra", Scope: "shared", RotationFormat: "numeric", ActiveFiles: []string{"app.log"}}}
	sources := registeredArchiveLogs(cfg)
	require.Len(t, sources, 3)
	for _, source := range sources {
		require.True(t, filepath.IsAbs(source.Directory), "archive registration must bind relative configuration: %s", source.ID)
		require.DirExists(t, source.Directory)
	}
	runtime, err := archiveops.RuntimeConfig(archiveops.Config{Directory: filepath.Join(root, "archive-state"), Instance: "dev", ExecutionNodeID: cfg.EnvironmentID, Sources: sources})
	require.NoError(t, err)
	svc := archiveops.NewService(context.Background(), runtime, archiveops.Options{ConfigError: err})
	t.Cleanup(svc.Close)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true}), true)
	view, err := svc.Overview(ctx)
	require.NoError(t, err, "relative log configuration must not disable the entire archive service")
	require.Len(t, view.Sources, 3)
	require.Empty(t, view.Policies, "registration must not enable transfer")
}

func TestNativePostgresRegistrationPreservesManifestWithoutEnablingTransfer(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir, manifest := filepath.Join(root, "log"), filepath.Join(root, "current_logfiles")
	require.NoError(t, os.Mkdir(dir, 0700))
	name := "postgresql-2026-01-01_010000.log"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("owned fixture"), 0600))
	require.NoError(t, os.WriteFile(manifest, []byte("stderr log/"+name+"\n"), 0600))
	cfg := &config.Config{EnvironmentID: "owned-native-install"}
	cfg.StorageGovernance.Sources = []config.StorageObserverSource{{ID: "shared-pg", Name: "Shared PostgreSQL logs", Kind: "postgresql_logs", Path: dir, Scope: "shared", RotationFormat: "postgresql", ActiveFiles: []string{"postgresql-"}, CurrentLogfiles: manifest}}
	sources := registeredArchiveLogs(cfg)
	require.Len(t, sources, 1)
	require.Equal(t, manifest, sources[0].CurrentLogfiles)
	runtime, err := archiveops.RuntimeConfig(archiveops.Config{Directory: filepath.Join(root, "archives"), Instance: "dev", ExecutionNodeID: cfg.EnvironmentID, Sources: sources})
	require.NoError(t, err)
	svc := archiveops.NewService(context.Background(), runtime, archiveops.Options{})
	t.Cleanup(svc.Close)
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true}), true)
	view, err := svc.Overview(ctx)
	require.NoError(t, err)
	require.Len(t, view.Sources, 1)
	require.Empty(t, view.Policies)
	require.Empty(t, view.Targets)
}
