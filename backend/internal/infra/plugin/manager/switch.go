package manager

import (
	"context"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func (m *managerImpl) SwitchVersion(ctx context.Context, id, version string, enable bool) (plugin_mgr.Plugin, error) {
	if id == "" || version == "" {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeInvalidArg, plugin_mgr.WithOp("switch_version"),
			plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(version),
			plugin_mgr.WithMsg("id or version empty"),
		)
	}

	// 1) 目标版本必须已安装
	if !m.opts.Registry.HasVersion(ctx, id, version) {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeNotFound, plugin_mgr.WithOp("switch_version"),
			plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(version),
			plugin_mgr.WithMsg("target version not installed"),
		)
	}
	target, ok := m.opts.Registry.GetVersion(ctx, id, version)
	if !ok {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeNotFound, plugin_mgr.WithOp("switch_version.database_binding"),
			plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(version),
		)
	}
	if err := m.validatePluginDatabaseBinding(id, target.HostConfig); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
			plugin_mgr.CodeConflict,
			err,
			plugin_mgr.WithOp("switch_version.database_binding"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(version),
		)
	}

	// 2) 如当前启用，先停
	if cur, ok := m.opts.Registry.Get(ctx, id); ok && cur.State == plugin_mgr.StateEnabled {
		if m.opts.TenantInstanceCount != nil {
			count, err := m.opts.TenantInstanceCount(ctx, id)
			if err != nil {
				return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
					plugin_mgr.CodeLifecycleError,
					err,
					plugin_mgr.WithOp("switch_version.tenant_instance_check"),
					plugin_mgr.WithPlugin(id),
					plugin_mgr.WithVersion(cur.Version),
				)
			}
			if count > 0 {
				return plugin_mgr.Plugin{}, plugin_mgr.NewError(
					plugin_mgr.CodeConflict,
					plugin_mgr.WithOp("switch_version.tenant_instance_check"),
					plugin_mgr.WithPlugin(id),
					plugin_mgr.WithVersion(cur.Version),
					plugin_mgr.WithMsg("plugin has %d active tenant instances; drain required before switching enabled runtime", count),
				)
			}
		}
		if err := m.Disable(ctx, id); err != nil {
			return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
				plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("switch_version.disable"))
		}
	}

	// 3) 切 current 指针
	if err := m.opts.Registry.SetCurrent(ctx, id, version); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
			plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("switch_version.set_current"))
	}
	if err := m.opts.Registry.Save(ctx); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
			plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("switch_version.save"))
	}

	// 4) 需要的话，启用新版本
	if enable {
		if err := m.Enable(ctx, id); err != nil {
			return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
				plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("switch_version.enable"))
		}
	}

	// 5) 返回当前视图（此时 current=version）
	if p, ok := m.opts.Registry.Get(ctx, id); ok {
		return p, nil
	}
	return plugin_mgr.Plugin{}, plugin_mgr.NewError(
		plugin_mgr.CodeInternal, plugin_mgr.WithOp("switch_version"),
		plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(version),
		plugin_mgr.WithMsg("switched but not found"))
}
