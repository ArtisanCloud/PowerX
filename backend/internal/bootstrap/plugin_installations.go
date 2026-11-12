package bootstrap

import (
	"context"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	notify "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/notify"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	install "github.com/ArtisanCloud/PowerX/internal/service/plugin_install"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func configurePluginInstallations(ctx context.Context, deps *shared.Deps, cfg *config.Config, mgr pm.Manager) {
	if deps.DB == nil || deps.EventFabric == nil || deps.EventFabric.StreamTasks == nil || deps.EventFabric.RedisClient == nil || cfg.Recovery.ReadOnly() {
		return
	}
	svc, e := install.NewService(deps.DB, install.Options{
		Disabled: config.BackgroundWorkersDisabled(cfg), RequireWorker: true,
		Root: filepath.Join(filepath.Dir(abs(cfg.Plugin.InstalledDir)), "install-tasks"), InstanceID: cfg.EnvironmentID, Secret: []byte(cfg.Server.SecretKey), Queue: deps.EventFabric.StreamTasks, Manager: mgr,
		Authorize: func(run context.Context, tenantUUID string, actor uint64) error {
			var user iam.User
			if e := deps.DB.WithContext(run).Where("id = ? AND is_root = ? AND status = ?", actor, true, iam.UserStatusActive).First(&user).Error; e != nil {
				return dto.NewErrorWithCode(403, "PLUGIN_INSTALL_ROOT_REQUIRED", "Root identity is no longer active", nil)
			}
			claims := reqctx.GetClaims(run)
			if claims == nil || claims.UserUUID != user.UUID.String() {
				return dto.NewErrorWithCode(403, "PLUGIN_INSTALL_ROOT_REQUIRED", "Root identity changed", nil)
			}
			var count int64
			if e := deps.DB.WithContext(run).Model(&tenant.Tenant{}).Where("uuid = ? AND status = ?", tenantUUID, 1).Count(&count).Error; e != nil || count != 1 {
				return dto.NewErrorWithCode(403, "PLUGIN_INSTALL_TENANT_UNAVAILABLE", "installation tenant is unavailable", nil)
			}
			return nil
		},
		Finalize: func(run context.Context, tenantUUID string, p pm.Plugin) error {
			svc := pluginservice.NewTenantPluginInstanceService(deps.DB).WithEventFabricNamespaceOwners(cfg.Plugin.EventFabricNamespaceOwners)
			_, cid, secret, e := svc.Enable(run, tenantUUID, p, nil)
			if e != nil {
				return e
			}
			if secret != "" {
				_ = notify.PushTenantCredentials(run, p.ID, tenantUUID, cid, secret)
			}
			bindings, e := setting.NewPluginInstanceConfigRepository(deps.DB).ListTenantPluginBindings(run, setting.ListTenantPluginOptions{PluginIDs: []string{p.ID}, Key: setting.KeyClientCredentials, OnlyEnabled: true})
			if e != nil {
				return e
			}
			for _, binding := range bindings {
				if _, _, _, e := svc.Enable(run, binding.TenantUUID, p, nil); e != nil {
					return e
				}
			}
			return nil
		},
		Notify: func(run context.Context, channel string) {
			_ = deps.EventFabric.RedisClient.Publish(run, channel, "changed").Err()
		},
	})
	if e != nil {
		logger.WarnF(ctx, "plugin installation tasks unavailable: %T", e)
		return
	}
	deps.PluginInstallations = svc
	if config.BackgroundWorkersDisabled(cfg) {
		return
	}
	go func() {
		if e := svc.Run(ctx); e != nil && ctx.Err() == nil {
			logger.WarnF(ctx, "plugin installation TaskBus stopped: %T", e)
		}
	}()
}
