package bootstrap

import (
	"context"
	"fmt"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/manifest"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/pluginbinding"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"gorm.io/gorm"
)

// Core 的准备事务统一供安装与租户启用使用，恢复只读核验。
func preparePluginEventFabric(ctx context.Context, deps *shared.Deps, p pm.Plugin, tenant, path string, provision bool, owners map[string]string, persist func(map[string]string) error) error {
	if deps == nil || deps.EventFabric == nil || deps.EventFabric.Directory == nil || deps.EventFabric.ACL == nil {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_SERVICE_REQUIRED")
	}
	return pluginbinding.NewService(deps.DB).WithNamespaceOwners(owners).Prepare(ctx, p, tenant, path, provision, func(_ *gorm.DB, bindings map[string]string) error { return persist(bindings) })
}

// 保留纯合同测试入口，实际运行装配使用同一个公共实现。
func pluginEventSeedPlan(p pm.Plugin, tenant, path string, existing map[string]string) (*manifest.SeedPlan, manifest.SeedContext, map[string][]string, error) {
	return pluginbinding.SeedPlan(p, tenant, path, existing)
}
func explicitEventBindings(p pm.Plugin) (map[string]string, error) {
	return pluginbinding.ExplicitBindings(p)
}
func verifyInstalledEventBindings(ctx context.Context, dir pluginbinding.Directory, acl pluginbinding.ACL, pluginID, tenant string, bindings map[string]string, actions map[string][]string) (map[string]string, error) {
	return pluginbinding.Verify(ctx, dir, acl, pluginID, tenant, bindings, actions)
}
