package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/bootstrap"
	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/autoseed"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/manifest"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value != "" {
		*s = append(*s, value)
	}
	return nil
}

type seedFlags struct {
	configPath   string
	manifestPath string
	dryRun       bool
	tenants      stringList
	plugins      stringList
}

type bindingTarget struct {
	TenantUUID string
	PluginID   string
}

type pluginCache struct {
	plugins map[string]pm.Plugin
}

func main() {
	flags := parseFlags()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(flags.configPath)
	if err != nil {
		fatalf("加载配置失败: %v", err)
	}
	config.GlobalConfig = cfg
	logger.InitGlobalLogger(&cfg.LogConfig)

	deps, err := bootstrap.BootstrapApp(ctx, cfg)
	if err != nil {
		fatalf("初始化核心依赖失败: %v", err)
	}
	defer closeDeps(deps)

	if deps.EventFabric == nil || deps.EventFabric.Seeder == nil {
		fatalf("Event Fabric Seeder 未初始化，请检查配置")
	}

	registry, err := loadPluginRegistry(ctx, cfg.Plugin.RegistryFile)
	if err != nil {
		fatalf("加载插件 Registry 失败: %v", err)
	}
	cache := pluginCache{plugins: map[string]pm.Plugin{}}
	for _, plugin := range registry.List(ctx) {
		cache.plugins[strings.TrimSpace(plugin.ID)] = plugin
	}

	if flags.manifestPath != "" && len(flags.plugins) != 1 {
		fatalf("使用 --manifest 时必须通过 --plugin 指定唯一插件 ID")
	}
	manifestOverride := ""
	if flags.manifestPath != "" {
		path, err := filepath.Abs(flags.manifestPath)
		if err != nil {
			fatalf("解析 manifest 路径失败: %v", err)
		}
		manifestOverride = path
	}

	targets, err := loadTargets(ctx, deps, flags.tenants, flags.plugins)
	if err != nil {
		fatalf("查询租户-插件绑定失败: %v", err)
	}
	if len(targets) == 0 {
		logger.InfoF(ctx, "没有匹配的租户-插件绑定，退出。")
		return
	}

	manifestCache := map[string]*manifest.Manifest{}
	successCount := 0
	skipCount := 0
	failCount := 0
	overridePlugin := ""
	if len(flags.plugins) == 1 && manifestOverride != "" {
		overridePlugin = flags.plugins[0]
	}

	for _, target := range targets {
		if overridePlugin != "" && !strings.EqualFold(target.PluginID, overridePlugin) {
			continue
		}
		plugin, ok := cache.plugins[target.PluginID]
		if !ok {
			logger.WarnF(ctx, "[SKIP] tenant=%s plugin=%s 不存在于 Registry", target.TenantUUID, target.PluginID)
			skipCount++
			continue
		}

		manifestPath := manifestOverride
		if manifestPath == "" {
			path, err := autoseed.ResolveManifestPath(plugin)
			if err != nil {
				logger.ErrorF(ctx, "[ERROR] tenant=%s plugin=%s 解析 manifest 失败: %v", target.TenantUUID, target.PluginID, err)
				failCount++
				continue
			}
			manifestPath = path
		}
		if manifestPath == "" {
			logger.WarnF(ctx, "[SKIP] tenant=%s plugin=%s 未发现 event_fabric manifest", target.TenantUUID, target.PluginID)
			skipCount++
			continue
		}

		doc, err := loadManifest(manifestPath, manifestCache)
		if err != nil {
			logger.ErrorF(ctx, "[ERROR] tenant=%s plugin=%s 加载 manifest 失败: %v", target.TenantUUID, target.PluginID, err)
			failCount++
			continue
		}

		seedCtx := manifest.SeedContext{
			TenantUUID:    target.TenantUUID,
			PluginID:      plugin.ID,
			PluginVersion: plugin.Version,
			Operator:      "event-fabric-seed-cli",
			Variables:     autoseed.BuildSeedVariables(plugin),
		}

		plan, err := doc.Render(seedCtx)
		if err != nil {
			logger.ErrorF(ctx, "[ERROR] tenant=%s plugin=%s 渲染 manifest 失败: %v", target.TenantUUID, target.PluginID, err)
			failCount++
			continue
		}

		logger.InfoF(ctx, "处理 tenant=%s plugin=%s manifest=%s topics=%d",
			target.TenantUUID, target.PluginID, manifestPath, len(plan.Topics))
		printPlan(ctx, plan)

		if flags.dryRun {
			logger.InfoF(ctx, "  [dry-run] 仅预览，不执行播种。")
			skipCount++
			continue
		}

		result, err := deps.EventFabric.Seeder.ApplyPlan(ctx, plan, seedCtx)
		if err != nil {
			logger.ErrorF(ctx, "[ERROR] tenant=%s plugin=%s 播种失败: %v", target.TenantUUID, target.PluginID, err)
			failCount++
			continue
		}
		createdTopics := 0
		grants := 0
		for _, topic := range result.Topics {
			if topic.Created {
				createdTopics++
			}
			grants += topic.GrantedActions
		}
		logger.InfoF(ctx, "  ✅ 完成：topics=%d created=%d grants=%d", len(result.Topics), createdTopics, grants)
		successCount++
	}

	logger.InfoF(ctx, "完成：success=%d skipped=%d failed=%d", successCount, skipCount, failCount)
	if failCount > 0 {
		os.Exit(1)
	}
}

func parseFlags() seedFlags {
	var flags seedFlags
	defaultConfigPath := strings.TrimSpace(os.Getenv("POWERX_CONFIG"))
	if defaultConfigPath == "" {
		defaultConfigPath = "etc/config.yaml"
	}
	flag.StringVar(&flags.configPath, "config", defaultConfigPath, "配置文件路径")
	flag.StringVar(&flags.manifestPath, "manifest", "", "自定义 manifest 路径（需配合 --plugin 单独使用）")
	flag.BoolVar(&flags.dryRun, "dry-run", false, "仅预览不执行播种")
	flag.Var(&flags.tenants, "tenant", "目标租户 UUID，可重复指定；省略则遍历全部")
	flag.Var(&flags.plugins, "plugin", "目标插件 ID，可重复指定；省略则遍历全部")
	flag.Parse()
	return flags
}

func loadTargets(ctx context.Context, deps *shared.Deps, tenants, plugins []string) ([]bindingTarget, error) {
	service := setting.NewPluginInstanceConfigService(deps)
	opts := reposetting.ListTenantPluginOptions{
		TenantUUIDs: tenants,
		PluginIDs:   plugins,
		Key:         setting.KeyClientCredentials,
		OnlyEnabled: true,
	}
	rows, err := service.ListTenantPluginBindings(ctx, opts)
	if err != nil {
		return nil, err
	}
	targets := make([]bindingTarget, 0, len(rows))
	for _, row := range rows {
		targets = append(targets, bindingTarget{
			TenantUUID: row.TenantUUID,
			PluginID:   row.PluginID,
		})
	}
	return targets, nil
}

func loadPluginRegistry(ctx context.Context, registryPath string) (pmimpl.Registry, error) {
	if strings.TrimSpace(registryPath) == "" {
		return nil, fmt.Errorf("cfg.plugin.registry_file 未配置")
	}
	path, err := filepath.Abs(registryPath)
	if err != nil {
		return nil, err
	}
	registry := pmimpl.NewJSONRegistry(path)
	if err := registry.Load(ctx); err != nil {
		return nil, err
	}
	return registry, nil
}

func loadManifest(path string, cache map[string]*manifest.Manifest) (*manifest.Manifest, error) {
	if doc, ok := cache[path]; ok {
		return doc, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	doc, err := manifest.Parse(file)
	if err != nil {
		return nil, err
	}
	cache[path] = doc
	return doc, nil
}

func printPlan(ctx context.Context, plan *manifest.SeedPlan) {
	for _, topic := range plan.Topics {
		logger.InfoF(ctx, "  - %s (namespace=%s acl=%d)", topic.FullTopic, topic.Topic.Namespace, len(topic.ACL))
	}
}

func closeDeps(deps *shared.Deps) {
	if deps == nil {
		return
	}
	if deps.AuditSvc != nil {
		deps.AuditSvc.Close()
	}
	closeSQL(deps.DB)
}

func closeSQL(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	if err = sqlDB.Close(); err != nil {
		logger.Warn(context.Background(), "关闭数据库连接失败: "+err.Error())
	}
}

func fatalf(format string, args ...any) {
	logger.ErrorF(context.Background(), format, args...)
	os.Exit(1)
}
