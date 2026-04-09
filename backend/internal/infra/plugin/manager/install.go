package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// InstallFromFile: 从本地目录安装插件（最小实现）
// 约定 srcDir 下包含 plugin.yaml，可能还包含 backend/bin/*、frontend/* 等
func (m *managerImpl) InstallFromFile(ctx context.Context, srcDir string, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	if srcDir == "" {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeInvalidArg, plugin_mgr.WithOp("install_file"), plugin_mgr.WithMsg("srcDir is empty"),
		)
	}
	absSrc, err := filepath.Abs(srcDir)
	if err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_file"))
	}

	// 1) 读取并合并 manifest（plugin.yaml + catalogs.*）
	man, err := loadManifestWithCatalogs(absSrc)
	if err != nil {
		return plugin_mgr.Plugin{}, err
	}
	if man.ID == "" || man.Version == "" {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeInvalidManifest, plugin_mgr.WithOp("install_file"), plugin_mgr.WithMsg("manifest.id or version empty"),
		)
	}
	// 安装阶段提前做一次完整校验，避免把坏包复制进 installed 目录。
	if err := NewFSLoader().Validate(ctx, man, absSrc); err != nil {
		return plugin_mgr.Plugin{}, err
	}

	// 2) 目标目录：<InstalledRoot>/<id>/<version>
	destRoot := filepath.Join(m.opts.InstalledRoot, man.ID, man.Version)
	if opts.Force {
		// 若 registry 中已有记录，走完整卸载流程（含停用与目录清理）
		if m.opts.Registry != nil && m.opts.Registry.HasVersion(ctx, man.ID, man.Version) {
			if err := m.UninstallAndPurge(ctx, man.ID, man.Version); err != nil {
				return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
					plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("install_file.force_uninstall"),
					plugin_mgr.WithPlugin(man.ID), plugin_mgr.WithVersion(man.Version),
				)
			}
		} else if _, err := os.Stat(destRoot); err == nil {
			if err := os.RemoveAll(destRoot); err != nil {
				return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
					plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_file.force_cleanup"), plugin_mgr.WithPath(destRoot),
				)
			}
		}
	}

	if _, err := os.Stat(destRoot); err == nil {
		// 若注册表尚未登记该版本，视为上次安装失败的残留，先清理再继续
		if m.opts.Registry != nil && !m.opts.Registry.HasVersion(ctx, man.ID, man.Version) {
			if err := os.RemoveAll(destRoot); err != nil {
				return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
					plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_file.cleanup"), plugin_mgr.WithPath(destRoot),
				)
			}
		} else {
			return plugin_mgr.Plugin{}, plugin_mgr.NewError(
				plugin_mgr.CodeAlreadyExists, plugin_mgr.WithOp("install_file"),
				plugin_mgr.WithPlugin(man.ID), plugin_mgr.WithVersion(man.Version),
				plugin_mgr.WithMsg("plugin version already installed"),
			)
		}
	}
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
			plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_file"), plugin_mgr.WithPath(destRoot),
		)
	}

	// 3) 拷贝整个目录
	if err := copyDir(absSrc, destRoot); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
			plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_file"),
			plugin_mgr.WithPath(fmt.Sprintf("%s -> %s", absSrc, destRoot)),
		)
	}
	if err := persistMergedManifest(destRoot, man); err != nil {
		return plugin_mgr.Plugin{}, err
	}

	// 4) 构造 Descriptor（注意：这里的 Descriptor 类型是 manager 包里的）
	paths := plugin_mgr.InstalledPaths{
		Root:              destRoot,
		FrontendAdminDir:  ResolvePath(destRoot, man.Frontend.Admin.StaticDir),
		Entry:             ResolvePath(destRoot, man.Runtime.Entry),
		PublicDir:         ResolvePath(destRoot, "public"),
		ContractsOpenAPI:  ResolvePath(destRoot, "contracts/openapi.yaml"),
		ContractsProtoDir: ResolvePath(destRoot, "contracts/proto"),
		ConfigDir:         ResolvePath(destRoot, "config"),
	}
	if man.Migrations != nil {
		paths.MigrationsDir = ResolvePath(destRoot, man.Migrations.Dir)
		paths.MigrationsEntry = ResolvePath(destRoot, man.Migrations.Entry)
		paths.MigrationsWorkDir = ResolvePath(destRoot, man.Migrations.WorkDir)
	}
	hostCfg, err := m.generateHostConfig(man, destRoot, opts.HostConfigSeed)
	if err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
			plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_file.host_config"),
			plugin_mgr.WithPlugin(man.ID), plugin_mgr.WithVersion(man.Version),
		)
	}
	if hostCfg != nil {
		paths.HostValuesFile = hostCfg.ValuesFile
	}
	desc := Descriptor{
		Manifest:        man,
		Paths:           paths,
		HostConfig:      hostCfg,
		InstallMetadata: opts.Metadata,
	}

	if rec, err := m.runPluginMigrate(ctx, desc, opts); err != nil {
		return plugin_mgr.Plugin{}, err
	} else if rec != nil {
		desc.Migration = rec
	}
	if m.opts.PostInstallManifest != nil {
		if err := m.opts.PostInstallManifest(ctx, man); err != nil {
			return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
				plugin_mgr.CodeInternal,
				err,
				plugin_mgr.WithOp("install_file.register_permissions"),
				plugin_mgr.WithPlugin(man.ID),
				plugin_mgr.WithVersion(man.Version),
			)
		}
	}

	// 5) 登记为 installed（Bootstrap 已处理“同版本跳过”，这里就是新装）
	if err := m.opts.Registry.Put(ctx, desc, plugin_mgr.StateInstalled); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("install_file"))
	}
	if err := m.opts.Registry.Save(ctx); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("install_file.save"))
	}

	// 6) 可选：安装后立即启用
	installedState := plugin_mgr.StateInstalled
	if opts.AutoEnable {
		// 安装后必须切到“本次安装版本”再启用，避免命中旧 current 版本导致 already_enabled。
		if _, err := m.SwitchVersion(ctx, man.ID, man.Version, true); err != nil {
			return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("install_file.enable"))
		}
		installedState = plugin_mgr.StateEnabled
	}

	// 7) 直接返回“刚安装的版本”视图（避免读 current 造成显示老版本）
	return plugin_mgr.Plugin{
		ID:              man.ID,
		Version:         man.Version,
		State:           installedState,
		Runtime:         man.Runtime,
		Frontend:        man.Frontend,
		Endpoints:       man.Endpoints,
		RBAC:            man.RBAC,
		Events:          man.Events,
		Paths:           desc.Paths,
		HostConfig:      hostCfg,
		InstallMetadata: opts.Metadata,
	}, nil
}

// 轻量目录拷贝（忽略 .git / .DS_Store）
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		base := filepath.Base(p)
		if base == ".git" || base == ".DS_Store" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
