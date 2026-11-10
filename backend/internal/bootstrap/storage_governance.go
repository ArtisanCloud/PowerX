package bootstrap

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	storageobserver "github.com/ArtisanCloud/PowerX/internal/infra/storage_observer"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	monitorlogs "github.com/ArtisanCloud/PowerX/internal/service/monitor_logs"
	snapshotops "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func configureStorageGovernance(cfg *config.Config, deps *shared.Deps, archives archiveops.Config, archiveError error, backupDirectory string) error {
	c := cfg.StorageGovernance
	if c.SchemaVersion != 0 && c.SchemaVersion != 1 {
		return fmt.Errorf("storage_governance.schema_version unsupported")
	}
	if c.Mode != "" && c.Mode != "observe" {
		return fmt.Errorf("storage_governance.mode only supports observe")
	}
	path := c.FilesystemPath
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("storage observation working directory unavailable")
		}
	}
	opts := storagegov.Options{
		LegacyHandoffBlocked: !c.AdvancedOperationsEnabled,
		Identity:             storagegov.Identity{HostID: c.HostID, InstanceID: cfg.EnvironmentID, Environment: cfg.Deployment.Env, FilesystemID: c.FilesystemID},
		FilesystemPath:       path, Collector: storageobserver.Collector{},
		DB:                    deps.DB,
		RequireRecoveryAnchor: cfg.Recovery.ProfileFile == "" && !cfg.Recovery.ReadOnly(),
		SamplingConfigured:    c.SamplingEnabled,
		SamplingWorkerEnabled: c.SamplingEnabled && !config.BackgroundWorkersDisabled(cfg),
	}
	if c.SamplingEnabled && (deps.DB == nil || cfg.EnvironmentID == "" || cfg.Deployment.Env == "") {
		return fmt.Errorf("storage sampling requires database and deployment identity")
	}
	if c.PostgresEnabled {
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`).MatchString(c.PostgresClusterID) {
			return fmt.Errorf("storage postgres metrics require registered cluster identity")
		}
		if deps.DB == nil || deps.DB.Dialector.Name() != "postgres" || cfg.EnvironmentID == "" || cfg.Deployment.Env == "" {
			return fmt.Errorf("storage postgres metrics require current PostgreSQL connection and deployment identity")
		}
		opts.DatabaseCollector = storageobserver.PostgresCollector{DB: deps.DB, Identity: opts.Identity, ClusterID: c.PostgresClusterID, Relations: []storageobserver.RelationSource{
			{ID: "capability-invocation-traces", Schema: coremodel.PowerXSchema, Name: "capability_registry_invocation_traces"},
			{ID: "capability-event-publications", Schema: coremodel.PowerXSchema, Name: "capability_registry_event_publications"},
		}}
	}
	// 内置来源由当前安装的实际运行配置生成；身份未就绪时不猜测 prod/dev。
	if cfg.EnvironmentID != "" && cfg.Deployment.Env != "" {
		add := func(id, name, kind, path string) {
			opts.Resources = append(opts.Resources, storagegov.Resource{ID: id, Name: name, Kind: kind, Path: path, Scope: cfg.Deployment.Env, InstanceID: cfg.EnvironmentID})
		}
		add("core-backups", "数据库备份文件", "backup_files", backupDirectory)
		snapDir := cfg.SystemSnapshots.Directory
		if snapDir == "" {
			snapDir = snapshotops.DefaultDirectory()
		}
		add("core-snapshots", "系统快照文件", "snapshot_files", snapDir)
		if archiveError == nil && archives.Directory != "" {
			add("core-archives", "归档对象与清单", "archive_objects", archives.Directory)
		}
	}
	for _, source := range c.Sources {
		opts.Resources = append(opts.Resources, storagegov.Resource{ID: source.ID, Name: source.Name, Kind: source.Kind, Scope: source.Scope, InstanceID: source.InstanceID, Path: source.Path, TenantUUID: source.TenantUUID})
	}
	// 使用实际日志配置，只扫描对应文件及正式轮转文件，不把 /tmp 等共享父目录全算给 PowerX。
	logs := &storageobserver.ApplicationLogs{Sources: map[string]storageobserver.ApplicationLogSource{}, LegacyEnabled: func() bool {
		if svc := monitorlogs.GetRetentionService(); svc != nil {
			return svc.Config().Enabled
		}
		// 维护启动/只读验收未启动旧 worker 时，也核对数据库中的有效旧策略。
		if deps.DB != nil && deps.DB.Dialector != nil {
			if svc := monitorlogs.NewRetentionService(cfg, deps.DB); svc != nil {
				return svc.Config().Enabled
			}
		}
		return cfg.LogConfig.Retention.Enabled
	}}
	if cfg.LogConfig.File.Enable && cfg.EnvironmentID != "" && cfg.Deployment.Env != "" {
		groups := map[string][]string{}
		for _, p := range []string{cfg.LogConfig.File.InfoFilePath, cfg.LogConfig.File.ErrorFilePath} {
			if p == "" {
				continue
			}
			abs, err := filepath.Abs(p)
			if err != nil {
				return err
			}
			dir := filepath.Dir(abs)
			dir = bindStorageSourcePath(dir)
			groups[dir] = append(groups[dir], filepath.Base(abs))
		}
		roots := make([]string, 0, len(groups))
		for dir := range groups {
			roots = append(roots, dir)
		}
		sort.Strings(roots)
		for i, dir := range roots {
			sort.Strings(groups[dir])
			id := fmt.Sprintf("core-app-logs-%x", sha256.Sum256([]byte(dir)))[:26]
			name := "应用日志"
			if len(roots) > 1 {
				name = fmt.Sprintf("应用日志（%d）", i+1)
			}
			opts.Resources = append(opts.Resources, storagegov.Resource{ID: id, Name: name, Kind: "application_logs", Path: dir, Scope: cfg.Deployment.Env, InstanceID: cfg.EnvironmentID, LogActiveFiles: groups[dir]})
			logs.Sources[id] = storageobserver.ApplicationLogSource{Directory: dir, ActiveFiles: groups[dir], RetentionDays: cfg.LogConfig.File.MaxAge, RetentionCount: cfg.LogConfig.File.MaxBackups, NativeManaged: c.ManageNativeRetention}
		}
	}
	opts.LogLifecycle = logs
	if cfg.EnvironmentID != "" && cfg.Deployment.Env != "" {
		observeDirectory := func(id, name, path string) error {
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			opts.Resources = append(opts.Resources, storagegov.Resource{ID: id, Name: name, Kind: "application_logs", Path: abs, Scope: cfg.Deployment.Env, InstanceID: cfg.EnvironmentID})
			return nil
		}
		if cfg.LogConfig.AgentDebug.Enable && cfg.LogConfig.AgentDebug.Dir != "" {
			if err := observeDirectory("core-agent-debug", "Agent 调试日志（仅观测）", cfg.LogConfig.AgentDebug.Dir); err != nil {
				return err
			}
		}
		if cfg.Audit.File.Enable {
			dir := cfg.Audit.File.Dir
			if dir == "" {
				dir = "logs/audit"
			}
			if err := observeDirectory("core-audit-files", "审计文件日志（仅观测）", dir); err != nil {
				return err
			}
		}
	}
	if cfg.Storage.Local.BasePath != "" && cfg.EnvironmentID != "" && cfg.Deployment.Env != "" {
		mediaRoot, err := filepath.Abs(cfg.Storage.Local.BasePath)
		if err != nil {
			return err
		}
		opts.Resources = append(opts.Resources, storagegov.Resource{ID: "core-local-media", Name: "本地媒体文件", Kind: "media_files", Path: mediaRoot, Scope: cfg.Deployment.Env, InstanceID: cfg.EnvironmentID})
	}
	for _, kind := range []string{"application_logs", "backup_files", "snapshot_files", "archive_objects", "media_files", "temporary_files", "postgresql_logs", "journal", "loki"} {
		state := "not_connected"
		for _, r := range opts.Resources {
			if r.Kind == kind {
				state = "connected"
				break
			}
		}
		if state != "connected" && kind == "journal" && runtime.GOOS != "linux" {
			state = "not_applicable"
		}
		if state != "connected" && kind == "application_logs" && !cfg.LogConfig.File.Enable {
			state = "disabled"
		}
		opts.Inventory = append(opts.Inventory, storagegov.CoverageItem{Kind: kind, State: state})
	}
	databaseState := "not_connected"
	if c.PostgresEnabled {
		databaseState = "connected"
	}
	opts.Inventory = append(opts.Inventory, storagegov.CoverageItem{Kind: "postgresql_database", State: databaseState}, storagegov.CoverageItem{Kind: "plugin_databases", State: "not_connected"})
	// 部署配置中的系统别名只在登记时解析一次；之后采集、清理仍使用不跟随软链的描述符。
	// macOS shell 的 PWD=/var/... 与实际 cwd=/private/var/... 必须绑定到同一个来源。
	for i := range opts.Resources {
		opts.Resources[i].Path = bindStorageSourcePath(opts.Resources[i].Path)
	}
	if err := configureStorageRuntime(cfg, deps, &opts, logs); err != nil {
		return err
	}
	if opts.Budget != nil && deps.DB != nil {
		controlOpts := storagegov.ControlOptions{Scope: storagegov.ControlScope{HostID: c.HostID, FilesystemID: c.FilesystemID, InstanceID: cfg.EnvironmentID}, Changed: func() {
			if deps.StorageGovernance != nil {
				deps.StorageGovernance.PublishChange("configuration_changed")
			}
		}}
		if deps.EventFabric != nil {
			controlOpts.Queue = deps.EventFabric.StreamTasks
		}
		control, err := storagegov.NewControlService(deps.DB, controlOpts)
		if err != nil {
			return err
		}
		opts.Control = control
		opts.ControlWorkerEnabled = !config.BackgroundWorkersDisabled(cfg)
	}
	svc, err := storagegov.NewService(opts)
	if err != nil {
		return err
	}
	deps.StorageGovernance = svc
	if opts.Control != nil || opts.SamplingConfigured {
		// 只读预检：保留登录、升级与 Root 诊断入口；依赖治理的调用继续失效关闭。
		check, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		checkDB := deps.DB
		if checkDB != nil && checkDB.Config != nil {
			checkDB = checkDB.WithContext(check)
		}
		if err := database.CheckStorageGovernanceSchema(checkDB); err != nil {
			logger.WarnF(check, "[storage-governance] migration/readiness required before dependent calls: %v; run the formal database upgrade/migrate", err)
		}
		cancel()
	}
	if deps.CapabilityRegistryAudit != nil {
		deps.CapabilityRegistryAudit.SetStorageAdmission(svc.CheckDatabaseAdmission)
	}
	return nil
}

// bindStorageSourcePath 固定可信部署配置的现存目标，不创建尚未生成的目录。
// 对缺失目录只解析现存父级；不可读、环路等错误留给采集器明确报告，不阻断业务启动。
func bindStorageSourcePath(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	parent := filepath.Clean(path)
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved
		}
		if !os.IsNotExist(err) || parent == filepath.Dir(parent) {
			return path
		}
		tail = append(tail, filepath.Base(parent))
		parent = filepath.Dir(parent)
	}
}
