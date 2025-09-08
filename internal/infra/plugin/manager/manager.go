package manager

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"sync"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// Options 注入依赖与基础配置
type Options struct {
	Enabled       bool
	BasePrefix    string
	InstalledRoot string
	RegistryFile  string

	Loader     Loader
	Registry   Registry
	HTTP       *router.DynamicRouter
	Supervisor *supervisor.Supervisor // 新增
}

// managerImpl 是内嵌版的具体实现（满足 plugin_mgr.Manager）
type managerImpl struct {
	mu   sync.RWMutex
	opts Options
	http *router.DynamicRouter
	sup  *supervisor.Supervisor // 新增
}

// New 生成一个内嵌管理器实现
func New(opts Options) plugin_mgr.Manager {
	m := &managerImpl{opts: opts}
	m.http = opts.HTTP
	m.sup = opts.Supervisor
	return m
}

func (m *managerImpl) Bootstrap(ctx context.Context) error {
	if !m.opts.Enabled {
		return nil
	}
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

		//// 4) 插件已存在，且版本相同，跳过
		if old, ok := m.opts.Registry.Get(ctx, id); ok && old.Version == ver {
			continue // ★ 关键：不要 Put 覆盖
		}
		if err := m.opts.Registry.Put(ctx, d, plugin_mgr.StateInstalled); err != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err,
				plugin_mgr.WithOp("bootstrap"),
				plugin_mgr.WithPlugin(id),
				plugin_mgr.WithVersion(ver),
			)
		}
	}

	// 4) 持久化一次（可选）
	if err := m.opts.Registry.Save(ctx); err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("bootstrap"))
	}

	return nil
}

func (m *managerImpl) Shutdown(ctx context.Context) error {
	// 预留：停止子进程、卸载路由、保存状态等
	return nil
}

// ------- 安装与升级（占位，后续里程碑实现） -------

func (m *managerImpl) Upgrade(ctx context.Context, id, version string, src plugin_mgr.InstallSource, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("upgrade"), plugin_mgr.WithMsg("not implemented"))
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
