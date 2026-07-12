package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
	if err := validateInstallPackagePlatform(man, absSrc); err != nil {
		return plugin_mgr.Plugin{}, err
	}

	// 2) 目标目录：<InstalledRoot>/<id>/<version>
	destRoot := filepath.Join(m.opts.InstalledRoot, man.ID, man.Version)
	if opts.Force {
		// Force 覆盖语义：替换运行产物，不应默认清理业务数据库资源。
		if m.opts.Registry != nil && m.opts.Registry.HasVersion(ctx, man.ID, man.Version) {
			if err := m.replaceInstalledVersion(ctx, man.ID, man.Version, destRoot); err != nil {
				return plugin_mgr.Plugin{}, err
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
			// 同版本重复安装按幂等处理：仍执行一次权限同步（upsert），再返回已安装版本。
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
			if m.opts.Registry != nil {
				if p, ok := m.opts.Registry.GetVersion(ctx, man.ID, man.Version); ok {
					return p, nil
				}
				if p, ok := m.opts.Registry.Get(ctx, man.ID); ok && p.Version == man.Version {
					return p, nil
				}
			}
			return plugin_mgr.Plugin{
				ID:          man.ID,
				Version:     man.Version,
				State:       plugin_mgr.StateInstalled,
				Runtime:     man.Runtime,
				Frontend:    man.Frontend,
				Endpoints:   man.Endpoints,
				Exposure:    man.Exposure,
				RBAC:        man.RBAC,
				Events:      man.Events,
				Backend:     man.Backend,
				Routes:      man.Routes,
				Permissions: append([]plugin_mgr.PermissionSpec(nil), man.Permissions...),
				Agents:      append([]plugin_mgr.AgentSpec(nil), man.Agents...),
				Tools:       append([]plugin_mgr.ToolSpec(nil), man.Tools...),
				Workflows:   append([]plugin_mgr.WorkflowSpec(nil), man.Workflows...),
				Paths: plugin_mgr.InstalledPaths{
					Root:              destRoot,
					FrontendAdminDir:  ResolvePath(destRoot, man.Frontend.Admin.StaticDir),
					Entry:             ResolvePath(destRoot, man.Runtime.Entry),
					PublicDir:         ResolvePath(destRoot, "public"),
					ContractsOpenAPI:  ResolvePath(destRoot, "contracts/openapi.yaml"),
					ContractsProtoDir: ResolvePath(destRoot, "contracts/proto"),
					ConfigDir:         ResolvePath(destRoot, "config"),
				},
				Name:        man.Name,
				Description: man.Description,
				Metadata:    man.Metadata,
			}, nil
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
	if err := ensureExecutableBits(destRoot); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(
			plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_file.chmod"),
			plugin_mgr.WithPath(destRoot),
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

func (m *managerImpl) replaceInstalledVersion(ctx context.Context, id, version, destRoot string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(version) == "" {
		return plugin_mgr.NewError(
			plugin_mgr.CodeInvalidArg,
			plugin_mgr.WithOp("install_file.force_replace"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(version),
			plugin_mgr.WithMsg("id or version empty"),
		)
	}
	if m == nil || m.opts.Registry == nil {
		return plugin_mgr.NewError(
			plugin_mgr.CodeInternal,
			plugin_mgr.WithOp("install_file.force_replace"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(version),
			plugin_mgr.WithMsg("registry not provided"),
		)
	}

	if cur, ok := m.opts.Registry.Get(ctx, id); ok && cur.Version == version && cur.State == plugin_mgr.StateEnabled {
		if m.opts.TenantInstanceCount != nil {
			count, err := m.opts.TenantInstanceCount(ctx, id)
			if err != nil {
				return plugin_mgr.Wrap(
					plugin_mgr.CodeLifecycleError,
					err,
					plugin_mgr.WithOp("install_file.force_replace.tenant_instance_check"),
					plugin_mgr.WithPlugin(id),
					plugin_mgr.WithVersion(version),
				)
			}
			if count > 0 {
				return plugin_mgr.NewError(
					plugin_mgr.CodeConflict,
					plugin_mgr.WithOp("install_file.force_replace.tenant_instance_check"),
					plugin_mgr.WithPlugin(id),
					plugin_mgr.WithVersion(version),
					plugin_mgr.WithMsg("plugin has %d active tenant instances; drain required before replacing enabled runtime", count),
				)
			}
		}
		if err := m.Disable(ctx, id); err != nil {
			return plugin_mgr.Wrap(
				plugin_mgr.CodeLifecycleError,
				err,
				plugin_mgr.WithOp("install_file.force_replace.disable"),
				plugin_mgr.WithPlugin(id),
				plugin_mgr.WithVersion(version),
			)
		}
	}

	if err := m.opts.Registry.Remove(ctx, id, version); err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeRegistryError,
			err,
			plugin_mgr.WithOp("install_file.force_replace.registry_remove"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(version),
		)
	}
	if err := m.opts.Registry.Save(ctx); err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeRegistryError,
			err,
			plugin_mgr.WithOp("install_file.force_replace.registry_save"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(version),
		)
	}

	if err := os.RemoveAll(destRoot); err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeIOError,
			err,
			plugin_mgr.WithOp("install_file.force_replace.cleanup"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(version),
			plugin_mgr.WithPath(destRoot),
		)
	}
	return nil
}

func validateInstallPackagePlatform(man plugin_mgr.Manifest, root string) error {
	candidates := executableCompatibilityCandidates(man)
	for _, candidate := range candidates {
		if err := validateExecutableCompatibility(root, candidate, man.ID, man.Version); err != nil {
			return err
		}
	}
	return nil
}

func executableCompatibilityCandidates(man plugin_mgr.Manifest) []string {
	seen := map[string]bool{}
	var out []string
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || !isPluginPackageExecutablePath(path) || seen[path] {
			return
		}
		seen[path] = true
		out = append(out, path)
	}
	if man.Runtime.Kind == plugin_mgr.RuntimeKindProcess {
		add(man.Runtime.Entry)
	}
	if man.Frontend.Admin.Kind == plugin_mgr.FrontendKindProcess && man.Frontend.Admin.Process != nil {
		add(man.Frontend.Admin.Process.Entry)
	}
	if man.Migrations != nil {
		add(man.Migrations.Entry)
		add(man.Migrations.RollbackEntry)
	}
	return out
}

func isPluginPackageExecutablePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if filepath.IsAbs(path) {
		return true
	}
	if strings.HasPrefix(path, ".") || strings.ContainsAny(path, `/\`) {
		return true
	}
	return false
}

func validateExecutableCompatibility(root, relPath, pluginID, version string) error {
	if filepath.IsAbs(relPath) {
		return plugin_mgr.NewError(
			plugin_mgr.CodeInvalidManifest,
			plugin_mgr.WithOp("install_file.platform_preflight"),
			plugin_mgr.WithPlugin(pluginID),
			plugin_mgr.WithVersion(version),
			plugin_mgr.WithPath(relPath),
			plugin_mgr.WithMsg("executable entry must be relative to plugin root"),
		)
	}
	cleanRel := filepath.Clean(relPath)
	if cleanRel == "." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return plugin_mgr.NewError(
			plugin_mgr.CodeInvalidManifest,
			plugin_mgr.WithOp("install_file.platform_preflight"),
			plugin_mgr.WithPlugin(pluginID),
			plugin_mgr.WithVersion(version),
			plugin_mgr.WithPath(relPath),
			plugin_mgr.WithMsg("executable entry escapes plugin root"),
		)
	}
	path := filepath.Join(root, cleanRel)
	info, err := os.Stat(path)
	if err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeMissingFile,
			err,
			plugin_mgr.WithOp("install_file.platform_preflight"),
			plugin_mgr.WithPlugin(pluginID),
			plugin_mgr.WithVersion(version),
			plugin_mgr.WithPath(cleanRel),
		)
	}
	if info.IsDir() {
		return plugin_mgr.NewError(
			plugin_mgr.CodeInvalidManifest,
			plugin_mgr.WithOp("install_file.platform_preflight"),
			plugin_mgr.WithPlugin(pluginID),
			plugin_mgr.WithVersion(version),
			plugin_mgr.WithPath(cleanRel),
			plugin_mgr.WithMsg("executable entry is a directory"),
		)
	}
	format, osName, arch, err := detectExecutableFormat(path)
	if err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeIOError,
			err,
			plugin_mgr.WithOp("install_file.platform_preflight"),
			plugin_mgr.WithPlugin(pluginID),
			plugin_mgr.WithVersion(version),
			plugin_mgr.WithPath(cleanRel),
		)
	}
	if format == "" {
		return nil
	}
	if osName != runtime.GOOS || arch != runtime.GOARCH {
		return plugin_mgr.NewError(
			plugin_mgr.CodeInvalidArg,
			plugin_mgr.WithOp("install_file.platform_preflight"),
			plugin_mgr.WithPlugin(pluginID),
			plugin_mgr.WithVersion(version),
			plugin_mgr.WithPath(cleanRel),
			plugin_mgr.WithMsg("incompatible executable %s: package target %s/%s (%s), host target %s/%s", cleanRel, osName, arch, format, runtime.GOOS, runtime.GOARCH),
		)
	}
	return nil
}

func detectExecutableFormat(path string) (format, osName, arch string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", "", err
	}
	defer f.Close()

	var hdr [20]byte
	n, err := f.Read(hdr[:])
	if err != nil && n == 0 {
		return "", "", "", err
	}
	if n < 4 {
		return "", "", "", nil
	}
	if hdr[0] == 0x7f && hdr[1] == 'E' && hdr[2] == 'L' && hdr[3] == 'F' {
		if n < 20 {
			return "elf", "", "", nil
		}
		machine := uint16(hdr[18]) | uint16(hdr[19])<<8
		switch machine {
		case 0x3e:
			return "elf", "linux", "amd64", nil
		case 0xb7:
			return "elf", "linux", "arm64", nil
		case 0x03:
			return "elf", "linux", "386", nil
		default:
			return "elf", "linux", fmt.Sprintf("machine-0x%x", machine), nil
		}
	}
	magicBE := uint32(hdr[0])<<24 | uint32(hdr[1])<<16 | uint32(hdr[2])<<8 | uint32(hdr[3])
	magicLE := uint32(hdr[3])<<24 | uint32(hdr[2])<<16 | uint32(hdr[1])<<8 | uint32(hdr[0])
	switch magicBE {
	case 0xfeedface, 0xfeedfacf:
		if n < 8 {
			return "mach-o", "darwin", "", nil
		}
		cpuType := uint32(hdr[4])<<24 | uint32(hdr[5])<<16 | uint32(hdr[6])<<8 | uint32(hdr[7])
		return "mach-o", "darwin", machoArch(cpuType), nil
	case 0xcafebabe, 0xcafebabf:
		return "mach-o-fat", "darwin", "universal", nil
	}
	switch magicLE {
	case 0xfeedface, 0xfeedfacf:
		if n < 8 {
			return "mach-o", "darwin", "", nil
		}
		cpuType := uint32(hdr[7])<<24 | uint32(hdr[6])<<16 | uint32(hdr[5])<<8 | uint32(hdr[4])
		return "mach-o", "darwin", machoArch(cpuType), nil
	}
	return "", "", "", nil
}

func machoArch(cpuType uint32) string {
	switch cpuType {
	case 0x01000007:
		return "amd64"
	case 0x0100000c:
		return "arm64"
	case 0x00000007:
		return "386"
	default:
		return fmt.Sprintf("cpu-0x%x", cpuType)
	}
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

func ensureExecutableBits(root string) error {
	for _, rel := range []string{filepath.Join("bin", "plugin"), filepath.Join("bin", "migrate")} {
		path := filepath.Join(root, rel)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.IsDir() {
			continue
		}
		if info.Mode()&0o111 == 0 {
			if err := os.Chmod(path, info.Mode()|0o755); err != nil {
				return err
			}
		}
	}
	return nil
}
