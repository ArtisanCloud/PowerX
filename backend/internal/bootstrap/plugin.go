package bootstrap

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"strings"
	"time"

	pmimplnotify "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/notify"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"os"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/config"
	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
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
				} else {
					logger.InfoF(ctx, "pushed credentials to plugin: plugin=%s tenant=%s", pluginID, tenantUUID)
				}
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
		enabled := 0
		for _, p := range list {
			logger.InfoF(ctx, "boot state: id=%s ver=%s state=%s", p.ID, p.Version, p.State)
			if p.State == pm.StateEnabled {
				enabled++
				if err := mgr.Enable(ctx, p.ID); err != nil {
					logger.WarnF(ctx, "auto-restore failed: id=%s err=%v", p.ID, err)
				} else {
					logger.InfoF(ctx, "auto-restore ok: id=%s", p.ID)
				}
			}
		}
		logger.InfoF(ctx, "auto-restore scanned=%d enabled=%d", len(list), enabled)
	}

	return mgr, nil
}
