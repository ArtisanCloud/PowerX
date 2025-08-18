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
// 约定 srcDir 目录结构包含 plugin.yaml、backend/bin/...（可选）、frontend/...（可选）
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
	var man plugin_mgr.Manifest // 你已有该结构（包含 Runtime/Frontend/Endpoints/RBAC/Events）
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

	// 2) 目标目录：plugins/installed/<id>/<version>
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

	// 4) 生成 Descriptor（用于登记）
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

	// 5) 登记为 installed（Bootstrap 已处理“跳过同版本”，这里是新装）
	if err := m.opts.Registry.Put(ctx, desc, plugin_mgr.StateInstalled); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("install_file"))
	}
	if err := m.opts.Registry.Save(ctx); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("install_file.save"))
	}

	// 6) （可选）安装后立即启用 —— 先借用 VerifyChecksum 当开关，后续你可以扩成 EnableAfterInstall
	if opts.VerifyChecksum {
		if err := m.Enable(ctx, man.ID); err != nil {
			return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("install_file.enable"))
		}
	}

	// 7) 返回当前版本视图
	if p, ok := m.opts.Registry.Get(ctx, man.ID); ok {
		return p, nil
	}
	return plugin_mgr.Plugin{}, plugin_mgr.NewError(
		plugin_mgr.CodeInternal, plugin_mgr.WithOp("install_file"), plugin_mgr.WithMsg("installed but not found in registry"),
	)
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
