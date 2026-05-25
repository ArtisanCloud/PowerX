package capability_sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capabilityregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// Options 描述 Capability Sync Worker 的运行参数。
type Options struct {
	ArtifactDir  string
	Watch        bool
	PollInterval time.Duration
}

// Runner 负责执行 Capability Sync Worker。
type Runner struct {
	deps   *shared.Deps
	opts   Options
	logger *pxlog.Logger
}

// NewRunner 构造一个新的 Worker。
func NewRunner(deps *shared.Deps, opts Options) *Runner {
	if opts.ArtifactDir == "" {
		opts.ArtifactDir = filepath.Clean("tmp/plugins")
	} else {
		opts.ArtifactDir = filepath.Clean(opts.ArtifactDir)
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 30 * time.Second
	}
	return &Runner{
		deps:   deps,
		opts:   opts,
		logger: pxlog.GetGlobalLogger(),
	}
}

// Run 根据 watch 配置执行一次或持续执行。
func (r *Runner) Run(ctx context.Context) error {
	if !r.opts.Watch {
		return r.runOnce(ctx)
	}

	r.logger.InfoF(ctx, "[capability_sync] watcher started (artifact_dir=%s, interval=%s)", r.opts.ArtifactDir, r.opts.PollInterval)

	ticker := time.NewTicker(r.opts.PollInterval)
	defer ticker.Stop()

	for {
		if err := r.runOnce(ctx); err != nil {
			r.logger.WarnF(ctx, "[capability_sync] sync pass failed: %v", err)
		}
		select {
		case <-ctx.Done():
			r.logger.InfoF(ctx, "[capability_sync] watcher stopped: %v", ctx.Err())
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) runOnce(ctx context.Context) error {
	if err := os.MkdirAll(r.opts.ArtifactDir, 0o755); err != nil {
		return fmt.Errorf("ensure artifact directory: %w", err)
	}

	syncWorker := capabilityregistry.NewSyncWorker(capabilityregistry.SyncWorkerConfig{
		DB:              r.deps.DB,
		Redis:           r.redisClient(),
		EventBus:        r.deps.EventBus,
		Logger:          r.logger,
		Audit:           r.deps.CapabilityRegistryAudit,
		Alerting:        r.deps.CapabilityRegistryAlerts,
		WorkflowCatalog: r.deps.WorkflowCatalog,
	})

	entries, err := os.ReadDir(r.opts.ArtifactDir)
	if err != nil {
		return fmt.Errorf("list artifact directory: %w", err)
	}

	artifactPaths, err := discoverArtifactPaths(r.opts.ArtifactDir, entries)
	if err != nil {
		return err
	}

	var errs []error
	for _, artifactPath := range artifactPaths {
		if err := syncWorker.ProcessArtifact(ctx, artifactPath); err != nil {
			r.logger.WarnF(ctx, "[capability_sync] artifact %s failed: %v", artifactPath, err)
			errs = append(errs, fmt.Errorf("%s: %w", artifactPath, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	r.logger.InfoF(ctx, "[capability_sync] processed %d artifact(s)", len(artifactPaths))
	return nil
}

func (r *Runner) redisClient() capabilityregistry.RedisClient {
	if r.deps == nil || r.deps.IntegrationGateway == nil {
		return nil
	}
	return r.deps.IntegrationGateway.RedisClient
}

func hasPluginManifest(dir string) bool {
	manifest := filepath.Join(dir, "plugin.yaml")
	if _, err := os.Stat(manifest); err == nil {
		return true
	}
	// 兼容插件将有效载荷放在 payload/ 下的情况
	if _, err := os.Stat(filepath.Join(dir, "payload", "plugin.yaml")); err == nil {
		return true
	}
	return false
}

func discoverArtifactPaths(artifactDir string, entries []os.DirEntry) ([]string, error) {
	artifactPaths := make([]string, 0, len(entries))
	for _, entry := range entries {
		artifactPath := filepath.Join(artifactDir, entry.Name())
		if !entry.IsDir() {
			if isArtifactFile(entry.Name()) {
				artifactPaths = append(artifactPaths, artifactPath)
			}
			continue
		}

		if hasPluginManifest(artifactPath) {
			artifactPaths = append(artifactPaths, artifactPath)
			continue
		}

		versionEntries, err := os.ReadDir(artifactPath)
		if err != nil {
			return nil, fmt.Errorf("list plugin version directory %s: %w", artifactPath, err)
		}
		for _, versionEntry := range versionEntries {
			if !versionEntry.IsDir() {
				continue
			}
			versionPath := filepath.Join(artifactPath, versionEntry.Name())
			if hasPluginManifest(versionPath) {
				artifactPaths = append(artifactPaths, versionPath)
			}
		}
	}
	return artifactPaths, nil
}

func isArtifactFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".pxp") || strings.HasSuffix(lower, ".zip")
}
