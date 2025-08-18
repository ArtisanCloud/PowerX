package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"gopkg.in/yaml.v3"
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

	// 1) 读取 manifest
	manifestPath := filepath.Join(absSrc, "plugin.yaml")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
			plugin_mgr.CodeMissingFile, err, plugin_mgr.WithOp("install_file"), plugin_mgr.WithPath(manifestPath),
		)
	}
	var man plugin_mgr.Manifest
	if err := yaml.Unmarshal(raw, &man); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
			plugin_mgr.CodeInvalidManifest, err, plugin_mgr.WithOp("install_file"), plugin_mgr.WithPath(manifestPath),
		)
	}
	if man.ID == "" || man.Version == "" {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeInvalidManifest, plugin_mgr.WithOp("install_file"), plugin_mgr.WithMsg("manifest.id or version empty"),
		)
	}

	// 2) 目标目录：<InstalledRoot>/<id>/<version>
	destRoot := filepath.Join(m.opts.InstalledRoot, man.ID, man.Version)
	if _, err := os.Stat(destRoot); err == nil {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeAlreadyExists, plugin_mgr.WithOp("install_file"),
			plugin_mgr.WithPlugin(man.ID), plugin_mgr.WithVersion(man.Version),
			plugin_mgr.WithMsg("plugin version already installed"),
		)
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

	// 4) 构造 Descriptor（注意：这里的 Descriptor 类型是 manager 包里的）
	desc := Descriptor{
		Manifest: man,
		Paths: plugin_mgr.InstalledPaths{
			Root:              destRoot,
			FrontendAdminDir:  ResolvePath(destRoot, man.Frontend.Admin.StaticDir),
			Entry:             ResolvePath(destRoot, man.Runtime.Entry),
			PublicDir:         ResolvePath(destRoot, "public"),
			MigrationsDir:     ResolvePath(destRoot, "migrations"),
			ContractsOpenAPI:  ResolvePath(destRoot, "contracts/openapi.yaml"),
			ContractsProtoDir: ResolvePath(destRoot, "contracts/proto"),
		},
	}

	// 5) 登记为 installed（Bootstrap 已处理“同版本跳过”，这里就是新装）
	if err := m.opts.Registry.Put(ctx, desc, plugin_mgr.StateInstalled); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("install_file"))
	}
	if err := m.opts.Registry.Save(ctx); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("install_file.save"))
	}

	// 6) 可选：安装后立即启用（此处临时复用 VerifyChecksum 作为开关）
	installedState := plugin_mgr.StateInstalled
	if opts.VerifyChecksum {
		if err := m.Enable(ctx, man.ID); err != nil {
			return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("install_file.enable"))
		}
		installedState = plugin_mgr.StateEnabled
	}

	// 7) 直接返回“刚安装的版本”视图（避免读 current 造成显示老版本）
	return plugin_mgr.Plugin{
		ID:        man.ID,
		Version:   man.Version,
		State:     installedState,
		Runtime:   man.Runtime,
		Frontend:  man.Frontend,
		Endpoints: man.Endpoints,
		RBAC:      man.RBAC,
		Events:    man.Events,
		Paths:     desc.Paths,
	}, nil
}

// 轻量目录拷贝（忽略 .git / .DS_Store）
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		base := filepath.Base(p)
		if base == ".git" || base == ".DS_Store" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
