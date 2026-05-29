package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// 逻辑卸载：停用（如在用）+ 从注册表移除 + Save
func (m *managerImpl) Uninstall(ctx context.Context, id string, versionOptional ...string) error {
	// 安全默认：UI/常规卸载不执行数据库 destructive 清理。
	return m.uninstall(ctx, false, id, versionOptional...)
}

// uninstall 允许调用方控制是否清理由宿主创建的数据库资源。
// clearDatabase=true: 保持历史行为（卸载时清理 schema/role）
// clearDatabase=false: 仅停用与注册表移除，不触碰数据库资源
func (m *managerImpl) uninstall(ctx context.Context, clearDatabase bool, id string, versionOptional ...string) error {
	if id == "" {
		return plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithOp("uninstall"), plugin_mgr.WithMsg("empty plugin id"))
	}
	if m != nil && m.opts.TenantInstanceCount != nil {
		count, err := m.opts.TenantInstanceCount(ctx, id)
		if err != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("uninstall.tenant_instance_check"), plugin_mgr.WithPlugin(id))
		}
		if count > 0 {
			return plugin_mgr.NewError(
				plugin_mgr.CodeConflict,
				plugin_mgr.WithOp("uninstall.tenant_instance_check"),
				plugin_mgr.WithPlugin(id),
				plugin_mgr.WithMsg("PLUGIN_HAS_TENANT_INSTANCES: plugin %s has %d tenant instances", id, count),
			)
		}
	}

	// 1) 目标版本
	targetVer := ""
	currentVer := ""
	if v, ok := m.opts.Registry.CurrentVersion(ctx, id); ok {
		currentVer = v
	}
	if len(versionOptional) > 0 && versionOptional[0] != "" {
		targetVer = versionOptional[0]
	} else {
		if currentVer != "" {
			v := currentVer
			targetVer = v
		} else {
			return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("uninstall"),
				plugin_mgr.WithPlugin(id), plugin_mgr.WithMsg("no current version to uninstall"))
		}
	}

	// 2) 必须已安装
	if !m.opts.Registry.HasVersion(ctx, id, targetVer) {
		return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("uninstall"),
			plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(targetVer), plugin_mgr.WithMsg("target version not installed"))
	}

	// 3) 若 current==target 且启用中，先停
	if cur, ok := m.opts.Registry.Get(ctx, id); ok {
		if cur.State == plugin_mgr.StateEnabled && cur.Version == targetVer {
			if err := m.Disable(ctx, id); err != nil {
				return plugin_mgr.Wrap(plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("uninstall.disable"),
					plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(targetVer))
			}
		}
	}

	// 4) 若删除当前版本，则连带清理该插件所有旧版本
	allVersions := m.opts.Registry.ListVersions(ctx, id)
	removeVersions := []string{targetVer}
	if currentVer != "" && targetVer == currentVer {
		if len(allVersions) > 0 {
			removeVersions = allVersions
		}
	}
	willRemoveAllVersions := len(allVersions) > 0 && len(removeVersions) == len(allVersions)

	// 5) 清理由宿主创建的数据库资源 + 从注册表删除
	for _, ver := range removeVersions {
		if pl, ok := m.opts.Registry.GetVersion(ctx, id, ver); ok {
			if clearDatabase {
				if err := m.cleanupPluginDatabaseResources(pl.HostConfig); err != nil {
					return plugin_mgr.Wrap(
						plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("uninstall.db_cleanup"),
						plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(ver),
					)
				}
			}
		}
		if err := m.opts.Registry.Remove(ctx, id, ver); err != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("uninstall.remove"),
				plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(ver))
		}
	}

	if err := m.opts.Registry.Save(ctx); err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("uninstall.save"),
			plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(targetVer))
	}

	// 6) 完整卸载插件后，回收插件来源权限（幂等）
	if willRemoveAllVersions && m.opts.PostUninstall != nil {
		if err := m.opts.PostUninstall(ctx, id); err != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("uninstall.revoke_permissions"),
				plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(targetVer))
		}
	}
	return nil
}

// 卸载并清理磁盘产物
func (m *managerImpl) UninstallAndPurge(ctx context.Context, id string, versionOptional ...string) error {
	if id == "" {
		return plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithOp("uninstall_purge"), plugin_mgr.WithMsg("empty plugin id"))
	}
	if m != nil && m.opts.TenantInstanceCount != nil {
		count, err := m.opts.TenantInstanceCount(ctx, id)
		if err != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("uninstall_purge.tenant_instance_check"), plugin_mgr.WithPlugin(id))
		}
		if count > 0 {
			return plugin_mgr.NewError(
				plugin_mgr.CodeConflict,
				plugin_mgr.WithOp("uninstall_purge.tenant_instance_check"),
				plugin_mgr.WithPlugin(id),
				plugin_mgr.WithMsg("PLUGIN_HAS_TENANT_INSTANCES: plugin %s has %d tenant instances", id, count),
			)
		}
	}

	// 先确定目标版本 & 路径（卸载前先拿路径）
	targetVer := ""
	currentVer := ""
	if v, ok := m.opts.Registry.CurrentVersion(ctx, id); ok {
		currentVer = v
	}
	if len(versionOptional) > 0 && versionOptional[0] != "" {
		targetVer = versionOptional[0]
	} else {
		if currentVer != "" {
			v := currentVer
			targetVer = v
		} else {
			return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("uninstall_purge"),
				plugin_mgr.WithPlugin(id), plugin_mgr.WithMsg("no current version to uninstall"))
		}
	}
	purgeVersions := []string{targetVer}
	if currentVer != "" && targetVer == currentVer {
		all := m.opts.Registry.ListVersions(ctx, id)
		if len(all) > 0 {
			purgeVersions = all
		}
	}
	paths := make([]string, 0, len(purgeVersions))
	for _, ver := range purgeVersions {
		pl, ok := m.opts.Registry.GetVersion(ctx, id, ver)
		if !ok {
			return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("uninstall_purge"),
				plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(ver), plugin_mgr.WithMsg("target version not installed"))
		}
		paths = append(paths, pl.Paths.Root)
	}

	// 先逻辑卸载
	// 即便 purge（磁盘产物清理）也不触发数据库 destructive 操作。
	if err := m.uninstall(ctx, false, id, targetVer); err != nil {
		return err
	}

	// 再安全删除产物目录
	for _, p := range paths {
		if err := m.purgePath(p); err != nil {
			return err
		}
	}
	return nil
}

// --- 内部工具 ---

func (m *managerImpl) purgePath(versionRoot string) error {
	root := filepath.Clean(m.opts.InstalledRoot)
	target := filepath.Clean(versionRoot)

	if !withinRoot(root, target) {
		return plugin_mgr.NewError(plugin_mgr.CodeIOError, plugin_mgr.WithOp("uninstall.purge"),
			plugin_mgr.WithPath(target), plugin_mgr.WithMsg("target outside installed root"))
	}
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("uninstall.purge"), plugin_mgr.WithPath(target))
	}
	// 尝试清空空目录（忽略错误）
	_ = os.Remove(filepath.Dir(target))               // .../<id>
	_ = os.Remove(filepath.Dir(filepath.Dir(target))) // .../installed
	return nil
}

func withinRoot(root, p string) bool {
	absRoot, _ := filepath.Abs(root)
	absP, _ := filepath.Abs(p)
	if absRoot == absP {
		return true
	}
	if !strings.HasSuffix(absRoot, string(os.PathSeparator)) {
		absRoot += string(os.PathSeparator)
	}
	return strings.HasPrefix(absP, absRoot)
}
