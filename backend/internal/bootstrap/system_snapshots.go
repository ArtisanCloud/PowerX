package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	gateway "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway"
	obsops "github.com/ArtisanCloud/PowerX/internal/service/observability_ops"
	snapshotops "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	recoveryops "github.com/ArtisanCloud/PowerX/internal/service/system_recovery"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

func configureSystemSnapshots(ctx context.Context, cfg *config.Config, deps *shared.Deps) {
	source := func(context.Context) (snapshotops.Config, error) {
		value, err := systemSnapshotSources(cfg)
		if err == nil {
			err = recoveryops.CompleteCurrentSource(cfg, config.GetGlobalConfigPath(), &value)
		}
		return value, err
	}
	// 来源尚未就绪的错误由 Service 在每次预检查时重新解析；不要将首次
	// 枚举失败固化，否则补齐构建或重新安装插件后仍必须重启才能恢复。
	initial, _ := source(ctx)
	dsn := cfg.Database.DSN
	if dsn == "" {
		dsn = composePostgresDSNFromDB(cfg.Database.Driver, cfg.Database.Host, cfg.Database.Port, cfg.Database.UserName, cfg.Database.Password, cfg.Database.Database, cfg.Database.SSLMode, cfg.Database.Timezone)
	}
	initial.DatabaseDSN = dsn
	if cfg.Recovery.ProfileFile != "" {
		profile, profileErr := recoveryops.ReadProfile(cfg.Recovery.ProfileFile)
		if profileErr != nil {
			initial.SourceProblem = "恢复协调器部署合同无效"
		} else {
			initial.Directory = profile.SnapshotDirectory
			initial.DirectorySource = "recovery_profile"
			initial.Instance = profile.InstanceID
			deps.SystemRecovery, profileErr = snapshotops.NewCoordinatorStore(profile.Directory, profile.Targets)
			if profileErr != nil {
				initial.SourceProblem = "恢复协调器队列不可用"
			}
		}
	}
	// 与预检查和恢复清单使用同一个已解析的媒体根；Core 配置允许
	// ./storage/media，但底层归档采集器只接受宿主提供的绝对路径。
	native := &snapshotops.PostgresBackend{DSN: dsn, MediaRoot: initial.MediaRoot, Restore: func(run context.Context, file string) (string, string, error) {
		run = reqctx.WithIsRoot(run, true)
		f, e := os.Open(file)
		if e != nil {
			return "", "", e
		}
		uploaded, e := deps.RestoreCopies.Import(run, "system-snapshot.dump", f)
		f.Close()
		if e != nil {
			return "", "", e
		}
		copy, e := deps.RestoreCopies.Create(run, backupops.CreateRestoreCopy{RestoreSource: backupops.RestoreSource{ImportID: uploaded.ID}, IdempotencyKey: uuid.NewString(), Operator: "system_snapshot"})
		if e != nil {
			return "", "", e
		}
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			row, e := deps.RestoreCopies.Get(run, copy.ID)
			if e != nil {
				return copy.ID, copy.Database, e
			}
			switch row.Status {
			case "success":
				return copy.ID, copy.Database, nil
			case "failed":
				return copy.ID, copy.Database, errors.New("数据库副本恢复失败，请查看关联副本核验报告")
			}
			select {
			case <-run.Done():
				return copy.ID, copy.Database, run.Err()
			case <-ticker.C:
			}
		}
	}}
	audit := obsops.NewUnifiedAuditWriter(deps.DB)
	options := snapshotops.Options{Backend: native, Sources: source, RecoveryStore: func() *snapshotops.CoordinatorStore { return deps.SystemRecovery }, Audit: func(run context.Context, event snapshotops.AuditEvent) error {
		return audit.Write(run, obsops.AuditRecord{ResourceType: "system_snapshot", ResourceID: event.ID, Operation: event.Action, Outcome: event.Outcome, Severity: "info", Detail: map[string]any{"operator": event.Operator, "trace_id": event.TraceID}})
	}}
	if deps.SystemRecovery != nil {
		options.DispatchCapture = func(run context.Context, id string) error {
			if !deps.SystemRecovery.Online() {
				return errors.New("恢复协调器未在线，请先启动独立服务")
			}
			// 补齐来源或重装插件后，协调器也必须收到最新的完整清单。
			current, e := source(run)
			if e != nil {
				return e
			}
			current.Directory = initial.Directory
			current.DirectorySource = initial.DirectorySource
			current.Instance = initial.Instance
			current.DatabaseDSN = dsn
			if e = deps.SystemSnapshots.SaveCoordinatorSource(current); e != nil {
				return e
			}
			return deps.SystemRecovery.EnqueueCapture(run, id)
		}
		options.Coordinator = true
	}
	deps.SystemSnapshots = snapshotops.NewService(ctx, initial, options)
	if deps.SystemRecovery == nil && !config.BackgroundWorkersDisabled(cfg) {
		// 普通启动直接登记当前实例，恢复时由同一 Core exec 进入维护模式。
		if current, err := recoveryops.NewCurrentRuntime(ctx, cfg, config.GetGlobalConfigPath(), initial, deps.SystemSnapshots); err == nil && current != nil {
			deps.CurrentRecovery = current
			deps.SystemRecovery = current.Coordinator.Store
		} else if err != nil {
			logger.WarnF(ctx, "当前实例维护恢复未启用: %v", err)
		}
	}
	if deps.SystemRecovery != nil {
		deps.SystemRecovery.SnapshotStore = deps.SystemSnapshots
		_ = deps.SystemSnapshots.SaveCoordinatorSource(initial)
	}
	if cfg.Recovery.ProfileFile == "" && !config.BackgroundWorkersDisabled(cfg) {
		deps.SystemSnapshots.StartScheduler()
	}
}

// 来源只从部署生效配置与允许的程序目录生成；每次采集重新枚举插件。
func systemSnapshotSources(cfg *config.Config) (snapshotops.Config, error) {
	directory, source := snapshotops.DefaultStorage()
	if cfg.SystemSnapshots.Directory != "" {
		directory = cfg.SystemSnapshots.Directory
		source = cfg.SystemSnapshots.DirectorySource()
	}
	out := snapshotops.Config{Directory: directory, DirectorySource: source, Instance: cfg.Deployment.Env, Version: cfg.Version, Platform: runtime.GOOS + "/" + runtime.GOARCH, MediaRoot: cfg.Storage.Local.BasePath, ConfigFiles: map[string]string{}, Artifacts: map[string]string{}}
	if cfg.Database.Driver != "postgres" && cfg.Database.Driver != "postgresql" {
		return out, errors.New("系统快照首版仅支持 PostgreSQL")
	}
	if cfg.Storage.DefaultDriver != "local" {
		return out, errors.New("系统快照首版需要本地媒体驱动")
	}
	media, e := filepath.Abs(out.MediaRoot)
	if e != nil || out.MediaRoot == "" {
		return out, errors.New("媒体根目录未配置")
	}
	// 即使启动时目录尚不存在，也保留解析结果；补齐目录后，动态
	// 预检查和已装配的采集器不能使用不同形式的来源路径。
	out.MediaRoot = media
	info, e := os.Lstat(media)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return out, errors.New("媒体根目录必须是存在的普通目录")
	}
	raw, e := yaml.Marshal(cfg)
	if e != nil {
		return out, e
	}
	var effective config.Config
	if e = yaml.Unmarshal(raw, &effective); e != nil {
		return out, e
	}
	effective.Storage.Local.BasePath = media
	if effective.Plugin.InstalledDir != "" {
		effective.Plugin.InstalledDir, _ = filepath.Abs(effective.Plugin.InstalledDir)
	}
	if effective.Plugin.RegistryFile != "" {
		effective.Plugin.RegistryFile, _ = filepath.Abs(effective.Plugin.RegistryFile)
	}
	raw, e = yaml.Marshal(&effective)
	if e != nil {
		return out, e
	}
	out.EffectiveConfig = raw
	executable, e := os.Executable()
	if e != nil {
		return out, e
	}
	executable, e = filepath.EvalSymlinks(executable)
	if e != nil {
		return out, e
	}
	out.Artifacts["core/powerx"] = executable
	root := os.Getenv("POWERX_LINKS_ROOT")
	if root == "" {
		cwd, e := os.Getwd()
		if e != nil {
			return out, e
		}
		root = cwd
		if _, e = os.Stat(filepath.Join(root, "web-admin")); errors.Is(e, os.ErrNotExist) {
			root = filepath.Dir(cwd)
		}
	} else if !filepath.IsAbs(root) {
		return out, errors.New("POWERX_LINKS_ROOT 必须是绝对路径")
	}
	out.Artifacts["core/web-admin"] = filepath.Join(root, "web-admin", ".output")
	if web := os.Getenv("POWERX_WEB_OUTPUT"); filepath.IsAbs(web) {
		out.Artifacts["core/web-admin"] = web
	}
	if info, e = os.Stat(filepath.Join(out.Artifacts["core/web-admin"], "server", "index.mjs")); e != nil || !info.Mode().IsRegular() {
		return out, errors.New("Web Admin 发布产物缺失，请先完成 npm run build")
	}
	capabilityDirectory, e := gateway.PlatformCapabilitiesDirectory()
	if e != nil {
		return out, errors.New("正式能力目录不可读取，请检查 PLATFORM_CAPABILITIES_DIR 后重新创建快照")
	}
	out.Artifacts["core/platform_capabilities"] = capabilityDirectory
	if cfg.Plugin.RegistryFile != "" {
		if _, e = os.Stat(cfg.Plugin.RegistryFile); e == nil {
			out.ConfigFiles["plugins/registry.json"] = cfg.Plugin.RegistryFile
		} else if !errors.Is(e, os.ErrNotExist) {
			return out, e
		}
	}
	installed := cfg.Plugin.InstalledDir
	if info, e := os.Stat(installed); e == nil && info.IsDir() {
		out.LifecycleRoots = append(out.LifecycleRoots, installed)
	}
	if installed == "" {
		return out, nil
	}
	ids, e := os.ReadDir(installed)
	if errors.Is(e, os.ErrNotExist) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	for _, id := range ids {
		if !id.IsDir() {
			continue
		}
		credential := filepath.Join(installed, id.Name(), ".runtime-credentials.json")
		if _, err := os.Lstat(credential); err == nil {
			out.ConfigFiles[filepath.ToSlash(filepath.Join("plugins", id.Name(), ".runtime-credentials.json"))] = credential
		} else if !errors.Is(err, os.ErrNotExist) {
			return out, err
		}
		versions, e := os.ReadDir(filepath.Join(installed, id.Name()))
		if e != nil {
			return out, e
		}
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			base := filepath.Join(installed, id.Name(), version.Name())
			logical := filepath.ToSlash(filepath.Join("plugins", id.Name(), version.Name()))
			for _, rel := range []string{"plugin.yaml", "backend/bin", "web-admin", "frontend", "public", "migrations", "contracts", "plugin.d", "skills"} {
				p := filepath.Join(base, rel)
				if _, e = os.Lstat(p); e == nil {
					out.Artifacts[logical+"/"+rel] = p
				} else if !errors.Is(e, os.ErrNotExist) {
					return out, e
				}
			}
			configRoot := filepath.Join(base, "config")
			if _, e = os.Stat(configRoot); errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e = filepath.WalkDir(configRoot, func(path string, d os.DirEntry, we error) error {
				if we != nil {
					return we
				}
				if d.IsDir() {
					return nil
				}
				rel, e := filepath.Rel(configRoot, path)
				if e != nil {
					return e
				}
				if strings.HasPrefix(rel, "..") {
					return errors.New("插件配置路径越界")
				}
				out.ConfigFiles[logical+"/config/"+filepath.ToSlash(rel)] = path
				return nil
			}); e != nil {
				return out, e
			}
		}
	}
	return out, nil
}

// LocalRecoverySources uses the same deployed source inventory as Core. The
// explicitly built Core replaces this CLI executable in the program component.
func LocalRecoverySources(cfg *config.Config, core string) (snapshotops.Config, error) {
	source, e := systemSnapshotSources(cfg)
	if e != nil {
		return source, e
	}
	source.Artifacts["core/powerx"] = core
	source.DatabaseDSN = cfg.Database.DSN
	if source.DatabaseDSN == "" {
		source.DatabaseDSN = composePostgresDSNFromDB(cfg.Database.Driver, cfg.Database.Host, cfg.Database.Port, cfg.Database.UserName, cfg.Database.Password, cfg.Database.Database, cfg.Database.SSLMode, cfg.Database.Timezone)
	}
	return source, nil
}
