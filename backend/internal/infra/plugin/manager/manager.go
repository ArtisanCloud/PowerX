package manager

import (
	"context"
	"strings"
	"sync"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

type PostInstallManifestHook func(ctx context.Context, manifest plugin_mgr.Manifest) error
type PostEnablePluginHook func(ctx context.Context, plugin plugin_mgr.Plugin, apiBaseURL string) error
type PostUninstallHook func(ctx context.Context, pluginID string) error
type TenantPluginInstanceChecker func(ctx context.Context, pluginID string) (int64, error)
type PluginRuntimeCredentialProvider func(ctx context.Context, pluginID string) (*PluginRuntimeCredential, error)

type PluginRuntimeCredential struct {
	TenantUUID     string
	ClientID       string
	ClientSecret   string
	GRPCAddress    string
	STSAudience    string
	STSScope       string
	GatewayBaseURL string
}

// Options 注入依赖与基础配置
type Options struct {
	Enabled       bool
	BasePrefix    string
	InstalledRoot string
	RegistryFile  string

	CoreConfig *config.Config

	Loader              Loader
	Registry            Registry
	HTTP                *router.DynamicRouter
	Supervisor          *supervisor.Supervisor
	PostInstallManifest PostInstallManifestHook
	PostEnablePlugin    PostEnablePluginHook
	PostUninstall       PostUninstallHook
	TenantInstanceCount TenantPluginInstanceChecker
	RuntimeCredential   PluginRuntimeCredentialProvider
}

// managerImpl 是内嵌版的具体实现（满足 plugin_mgr.Manager）
type managerImpl struct {
	mu   sync.RWMutex
	opts Options
	http *router.DynamicRouter
	sup  *supervisor.Supervisor // 新增

	// 内部通信令牌：pluginID -> token（仅内存，不落盘）
	tokens map[string]string
}

// New 生成一个内嵌管理器实现
func New(opts Options) plugin_mgr.Manager {
	m := &managerImpl{opts: opts, tokens: make(map[string]string)}
	m.http = opts.HTTP
	m.sup = opts.Supervisor
	return m
}

func (m *managerImpl) Bootstrap(ctx context.Context) error {
	// 1) 兜底：必须有 Loader/Registry
	if m.opts.Loader == nil || m.opts.Registry == nil {
		return plugin_mgr.NewError(plugin_mgr.CodeInternal,
			plugin_mgr.WithOp("bootstrap"),
			plugin_mgr.WithMsg("loader/registry not provided"),
		)
	}

	// 2) 先加载已有 registry 快照（文件不存在视为空）
	if err := m.opts.Registry.Load(ctx); err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("bootstrap"))
	}

	// 3) 扫描磁盘上的已安装插件目录，填充/刷新注册表（状态先标记为 installed）
	descs, err := m.opts.Loader.Discover(ctx, m.opts.InstalledRoot)
	if err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("bootstrap"))
	}

	for _, d := range descs {
		id := d.Manifest.ID
		ver := d.Manifest.Version

		// —— 新增：观测 manifest 里到底有哪些菜单声明
		adminMenus := 0
		if d.Manifest.Frontend.Admin.Menus != nil {
			adminMenus = len(d.Manifest.Frontend.Admin.Menus) // frontend.admin.menus
		}
		logger.InfoF(ctx, "[plugin-bootstrap] discover id=%s ver=%s admin=%d admin.static_dir=%q",
			id, ver, adminMenus, d.Paths.FrontendAdminDir)

		prevState := plugin_mgr.StateInstalled
		if old, ok := m.opts.Registry.Get(ctx, id); ok && old.Version == ver {
			prevState = old.State // 同版本覆盖 manifest/paths，保留状态（你已应用 C2 的逻辑）
		}

		if err := m.opts.Registry.Put(ctx, d, prevState); err != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err,
				plugin_mgr.WithOp("bootstrap"),
				plugin_mgr.WithPlugin(id),
				plugin_mgr.WithVersion(ver),
			)
		}
		// 启动扫描时补做一次权限同步（upsert 幂等），修复历史安装遗漏。
		if m.opts.PostInstallManifest != nil {
			if err := m.opts.PostInstallManifest(ctx, d.Manifest); err != nil {
				wrapped := plugin_mgr.Wrap(plugin_mgr.CodeInternal, err,
					plugin_mgr.WithOp("bootstrap.register_permissions"),
					plugin_mgr.WithPlugin(id),
					plugin_mgr.WithVersion(ver),
				)
				logger.WarnF(ctx, "[plugin-bootstrap] plugin permission sync failed, keep manager available: id=%s ver=%s err=%v", id, ver, wrapped)
				continue
			}
		}
	}

	// 4) 持久化一次（可选）
	if err := m.opts.Registry.Save(ctx); err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("bootstrap"))
	}

	if !m.opts.Enabled {
		// 插件系统处于禁用模式，仅同步注册表快照
		return nil
	}
	return nil
}

func (m *managerImpl) restoreEnabledPlugins(ctx context.Context) error {
	if m.opts.Registry == nil {
		return plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("bootstrap.restore_enabled"), plugin_mgr.WithMsg("registry not provided"))
	}
	for _, plugin := range m.opts.Registry.List(ctx) {
		if plugin.State != plugin_mgr.StateEnabled {
			continue
		}
		logger.InfoF(ctx, "[plugin-bootstrap] restore enabled plugin id=%s ver=%s", plugin.ID, plugin.Version)
		if err := m.Enable(ctx, plugin.ID); err != nil {
			wrapped := plugin_mgr.Wrap(plugin_mgr.CodeLifecycleError, err,
				plugin_mgr.WithOp("bootstrap.restore_enabled"),
				plugin_mgr.WithPlugin(plugin.ID),
				plugin_mgr.WithVersion(plugin.Version),
			)
			logger.ErrorF(ctx, "[plugin-bootstrap] restore enabled plugin failed, keep core available: id=%s ver=%s err=%v", plugin.ID, plugin.Version, wrapped)
			continue
		}
	}
	return nil
}

func (m *managerImpl) Shutdown(ctx context.Context) error {
	// 预留：停止子进程、卸载路由、保存状态等
	return nil
}

// ------- 安装与升级（占位，后续里程碑实现） -------

func (m *managerImpl) Upgrade(ctx context.Context, id, version string, src plugin_mgr.InstallSource, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	id = strings.TrimSpace(id)
	version = strings.TrimSpace(version)
	if id == "" {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeInvalidArg,
			plugin_mgr.WithOp("upgrade"),
			plugin_mgr.WithMsg("plugin id is empty"),
		)
	}
	if m.opts.Registry == nil {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeInternal,
			plugin_mgr.WithOp("upgrade"),
			plugin_mgr.WithMsg("registry not provided"),
		)
	}

	current, ok := m.opts.Registry.Get(ctx, id)
	if !ok {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeNotFound,
			plugin_mgr.WithOp("upgrade"),
			plugin_mgr.WithPlugin(id),
		)
	}

	if version != "" && !opts.Force && m.opts.Registry.HasVersion(ctx, id, version) {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeAlreadyExists,
			plugin_mgr.WithOp("upgrade"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(version),
			plugin_mgr.WithMsg("target version already installed"),
		)
	}

	installOpts := opts
	installOpts.AutoEnable = false
	installOpts.HostConfigSeed = cloneHostConfig(current.HostConfig)

	var (
		installed plugin_mgr.Plugin
		err       error
	)
	switch {
	case strings.TrimSpace(src.LocalFile) != "":
		installed, err = m.InstallFromFile(ctx, src.LocalFile, installOpts)
	case strings.TrimSpace(src.RemoteURL) != "":
		installed, err = m.InstallFromURL(ctx, src.RemoteURL, src.SHA256, src.Signature, installOpts)
	default:
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeInvalidArg,
			plugin_mgr.WithOp("upgrade"),
			plugin_mgr.WithMsg("install source not provided"),
		)
	}
	if err != nil {
		return plugin_mgr.Plugin{}, err
	}

	if installed.ID != id {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeInvalidManifest,
			plugin_mgr.WithOp("upgrade"),
			plugin_mgr.WithPlugin(installed.ID),
			plugin_mgr.WithMsg("manifest id mismatch"),
		)
	}
	if version != "" && installed.Version != version {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeInvalidManifest,
			plugin_mgr.WithOp("upgrade"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(installed.Version),
			plugin_mgr.WithMsg("manifest version mismatch"),
		)
	}

	enableNew := current.State == plugin_mgr.StateEnabled || opts.AutoEnable
	upgraded, err := m.SwitchVersion(ctx, id, installed.Version, enableNew)
	if err != nil {
		return plugin_mgr.Plugin{}, err
	}
	return upgraded, nil
}

// ------- 查询（先走 Registry，保证可用） -------

func (m *managerImpl) List(ctx context.Context) ([]plugin_mgr.Plugin, error) {
	if m.opts.Registry == nil {
		return nil, plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("list"), plugin_mgr.WithMsg("registry not provided"))
	}
	plugs := m.opts.Registry.List(ctx)
	return plugs, nil
}

func (m *managerImpl) Get(ctx context.Context, id string) (plugin_mgr.Plugin, error) {
	if m.opts.Registry == nil {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("get"), plugin_mgr.WithMsg("registry not provided"))
	}
	if p, ok := m.opts.Registry.Get(ctx, id); ok {
		return p, nil
	}
	return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("get"), plugin_mgr.WithPlugin(id))
}
