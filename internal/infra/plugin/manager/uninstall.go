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
	if id == "" {
		return plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithOp("uninstall"), plugin_mgr.WithMsg("empty plugin id"))
	}

	// 1) 目标版本
	targetVer := ""
	if len(versionOptional) > 0 && versionOptional[0] != "" {
		targetVer = versionOptional[0]
	} else {
		if v, ok := m.opts.Registry.CurrentVersion(ctx, id); ok {
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

	// 4) 清理由宿主创建的数据库资源
	if pl, ok := m.opts.Registry.GetVersion(ctx, id, targetVer); ok {
		if err := m.cleanupPluginDatabaseResources(pl.HostConfig); err != nil {
			return plugin_mgr.Wrap(
				plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("uninstall.db_cleanup"),
				plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(targetVer),
			)
		}
	}

	// 5) 从注册表删除并保存
	if err := m.opts.Registry.Remove(ctx, id, targetVer); err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("uninstall.remove"),
			plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(targetVer))
	}
	if err := m.opts.Registry.Save(ctx); err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("uninstall.save"),
			plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(targetVer))
	}
	return nil
}

// 卸载并清理磁盘产物
func (m *managerImpl) UninstallAndPurge(ctx context.Context, id string, versionOptional ...string) error {
	if id == "" {
		return plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithOp("uninstall_purge"), plugin_mgr.WithMsg("empty plugin id"))
	}

	// 先确定目标版本 & 路径（卸载前先拿路径）
	targetVer := ""
	if len(versionOptional) > 0 && versionOptional[0] != "" {
		targetVer = versionOptional[0]
	} else {
		if v, ok := m.opts.Registry.CurrentVersion(ctx, id); ok {
			targetVer = v
		} else {
			return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("uninstall_purge"),
				plugin_mgr.WithPlugin(id), plugin_mgr.WithMsg("no current version to uninstall"))
		}
	}

	pl, ok := m.opts.Registry.GetVersion(ctx, id, targetVer)
	if !ok {
		return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("uninstall_purge"),
			plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(targetVer), plugin_mgr.WithMsg("target version not installed"))
	}

	// 先逻辑卸载
	if err := m.Uninstall(ctx, id, targetVer); err != nil {
		return err
	}

	// 再安全删除产物目录
	return m.purgePath(pl.Paths.Root)
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
