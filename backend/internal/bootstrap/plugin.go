package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	pmimplnotify "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/notify"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/autoseed"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/manifest"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
)

// ---- 一个最小可跑的 Authorizer 占位实现 ----
// 生产请替换为 DB/gRPC 版本
type devAuthorizer struct{}

var superRoleCodes = map[iam.RoleCode]struct{}{
	iam.CodeSystemAdmin: {},
	iam.CodeRoleAdmin:   {},
}

func (devAuthorizer) Permissions(ctx context.Context, tenantID, userID uint64, pluginID string) ([]string, string, error) {
	// 开发期：只给读权限（按需改为 "*" 或 "note:*"）
	return []string{"*"}, "", nil
}
func (devAuthorizer) IsSuperAdmin(_ context.Context, _, _ uint64, roles []string) bool {
	for _, r := range roles {
		rc := iam.RoleCode(strings.ToLower(strings.TrimSpace(r)))
		if _, ok := superRoleCodes[rc]; ok {
			return true
		}
	}
	return false
}

func abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	wd, _ := os.Getwd()
	return filepath.Clean(filepath.Join(wd, p))
}

func BootstrapPlugin(ctx context.Context, deps *shared.Deps, cfg *config.Config, r *gin.Engine) (pm.Manager, error) {
	pluginAuth := middleware.JwtMiddleware(
		[]byte(cfg.Auth.JWTSecret),
		cfg.Auth.Issuer,
		[]string{cfg.Auth.AudienceUser},
		[]string{"access"},
		nil,
		middleware.WithTenantHeaderPolicy(middleware.TenantHeaderPolicy{RequireUUID: cfg.Tenants.RequireUUID}),
	)
	dr := router.NewDynamicRouter(cfg.Plugin.BasePrefix, r, pluginAuth)
	if sec := strings.TrimSpace(cfg.Auth.JWTSecret); sec != "" {
		dr.SetContextHMACSecret([]byte(sec))
	}
	sup := supervisor.New()

	installedRoot := abs(cfg.Plugin.InstalledDir)
	registryFile := abs(cfg.Plugin.RegistryFile)

	mgr := pmimpl.New(pmimpl.Options{
		Enabled:       cfg.Plugin.Enabled,
		BasePrefix:    cfg.Plugin.BasePrefix,
		InstalledRoot: installedRoot,
		RegistryFile:  registryFile,
		CoreConfig:    cfg,
		Loader:        pmimpl.NewFSLoader(),
		Registry:      pmimpl.NewJSONRegistry(registryFile),
		HTTP:          dr,
		Supervisor:    sup,
		PostInstallManifest: func(ctx context.Context, manifest pm.Manifest) error {
			return syncPluginManifestPermissions(ctx, deps.DB, manifest)
		},
		PostEnable: func(ctx context.Context, tenantUUID, pluginID string) error {
			svc := setting.NewPluginInstanceConfigService(deps)

			// 注意：EnsureCredentials 有 3 个返回值
			clientID, clientSecret, err := svc.EnsureCredentials(ctx, tenantUUID, pluginID, nil)
			if err != nil {
				return err
			}

			// 推送到插件（若首次创建有明文 secret）
			if clientSecret != "" {
				if err := pmimplnotify.PushTenantCredentials(ctx, pluginID, tenantUUID, clientID, clientSecret); err != nil {
					logger.WarnF(ctx, "push credentials to plugin failed: plugin=%s tenant=%s err=%v", pluginID, tenantUUID, err)
					return err
				} else {
					logger.InfoF(ctx, "pushed credentials to plugin: plugin=%s tenant=%s", pluginID, tenantUUID)
				}
			}

			if err := seedPluginEventFabric(ctx, deps, tenantUUID, pluginID); err != nil {
				return err
			}

			return nil
		},
	})
	if err := mgr.Bootstrap(ctx); err != nil {
		return nil, err
	}

	pmimpl.InitGlobal(mgr)
	if !cfg.Plugin.Enabled {
		return mgr, nil
	}

	// ★ 绑定 Authorizer（issuer/ttl 可配）
	pmimpl.BindAuthorizer(dr, devAuthorizer{}, "powerx-auth", 60*time.Second)

	// ★ 为每个已知插件安装策略（基于 HTTPBasePath + RBAC.Resources）
	if list, err := mgr.List(ctx); err == nil {
		for _, p := range list {
			pol := pmimpl.PolicyFromPlugin(p)
			pmimpl.InstallPolicy(dr, p.ID, pol)
		}
	} else {
		logger.WarnF(ctx, "install policies: list failed: %v", err)
	}

	// ★ 自动恢复：把上次 state=enabled 的插件重新启用
	if list, err := mgr.List(ctx); err != nil {
		logger.WarnF(ctx, "auto-restore: list failed: %v", err)
	} else {
		enabledPlugins := make([]pm.Plugin, 0, len(list))
		for _, p := range list {
			logger.InfoF(ctx, "boot state: id=%s ver=%s state=%s", p.ID, p.Version, p.State)
			if p.State == pm.StateEnabled {
				enabledPlugins = append(enabledPlugins, p)
			}
		}
		parallelism := resolvePluginAutoRestoreParallelism(cfg.Plugin.AutoRestoreParallelism)
		if parallelism > len(enabledPlugins) {
			parallelism = len(enabledPlugins)
		}

		logger.InfoF(ctx, "auto-restore scanned=%d enabled=%d parallelism=%d", len(list), len(enabledPlugins), parallelism)

		if parallelism <= 1 || len(enabledPlugins) <= 1 {
			for _, p := range enabledPlugins {
				if err := mgr.Enable(ctx, p.ID); err != nil {
					logger.WarnF(ctx, "auto-restore failed: id=%s err=%v", p.ID, err)
				} else {
					logger.InfoF(ctx, "auto-restore ok: id=%s", p.ID)
				}
			}
		} else {
			jobs := make(chan pm.Plugin, len(enabledPlugins))
			var wg sync.WaitGroup
			for i := 0; i < parallelism; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for p := range jobs {
						if err := mgr.Enable(ctx, p.ID); err != nil {
							logger.WarnF(ctx, "auto-restore failed: id=%s err=%v", p.ID, err)
						} else {
							logger.InfoF(ctx, "auto-restore ok: id=%s", p.ID)
						}
					}
				}()
			}
			for _, p := range enabledPlugins {
				jobs <- p
			}
			close(jobs)
			wg.Wait()
		}
	}

	return mgr, nil
}

func resolvePluginAutoRestoreParallelism(cfgValue int) int {
	// 优先级：env > config.yaml > default(1)
	n := cfgValue
	if n < 1 {
		n = 1
	}

	// 兼容项目前缀环境变量与历史变量
	raw := strings.TrimSpace(os.Getenv("CORE_X_PLUGIN_AUTORESTORE_PARALLELISM"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("PX_PLUGIN_AUTORESTORE_PARALLELISM"))
	}
	if raw != "" {
		if envN, err := strconv.Atoi(raw); err == nil && envN > 0 {
			n = envN
		}
	}

	// 防止误配置过大并发导致本机抖动
	if n > 8 {
		return 8
	}
	return n
}

func seedPluginEventFabric(ctx context.Context, deps *shared.Deps, tenantUUID, pluginID string) error {
	if deps == nil || deps.EventFabric == nil || deps.EventFabric.Seeder == nil {
		return nil
	}
	manager := pmimpl.GetPluginManager()
	if manager == nil {
		return fmt.Errorf("plugin manager is not initialized")
	}
	plugin, err := manager.Get(ctx, pluginID)
	if err != nil {
		return fmt.Errorf("load plugin %s: %w", pluginID, err)
	}
	manifestPath, err := autoseed.ResolveManifestPath(plugin)
	if err != nil {
		return fmt.Errorf("resolve event manifest: %w", err)
	}
	if manifestPath == "" {
		logger.DebugF(ctx, "[plugin-bootstrap] no event_fabric manifest found for plugin=%s", pluginID)
		return nil
	}

	file, err := os.Open(manifestPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", manifestPath, err)
	}
	defer file.Close()

	doc, err := manifest.Parse(file)
	if err != nil {
		return fmt.Errorf("parse %s: %w", manifestPath, err)
	}

	seedCtx := manifest.SeedContext{
		TenantUUID:    tenantUUID,
		PluginID:      plugin.ID,
		PluginVersion: plugin.Version,
		Operator:      "plugin-bootstrap",
		Variables:     autoseed.BuildSeedVariables(plugin),
	}
	if _, err := deps.EventFabric.Seeder.ApplyManifest(ctx, doc, seedCtx); err != nil {
		return err
	}
	logger.InfoF(ctx, "[plugin-bootstrap] event_fabric seeded plugin=%s tenant=%s manifest=%s", pluginID, tenantUUID, manifestPath)
	return nil
}
