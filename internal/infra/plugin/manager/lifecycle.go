package manager

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"net/url"
	"os"
	"path/filepath"
)

func (m *managerImpl) Enable(ctx context.Context, id string) error {
	p, err := m.mustGet(ctx, id)
	if err != nil {
		return err
	}
	if m.http == nil {
		return plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("enable"), plugin_mgr.WithMsg("dynamic router not initialized"))
	}

	// 1) Admin 静态（存在就挂）
	if p.Frontend.Admin.Kind == plugin_mgr.FrontendKindStatic && p.Paths.FrontendAdminDir != "" {
		absDir, err := filepath.Abs(p.Paths.FrontendAdminDir)
		if err != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
		}
		if fi, err := os.Stat(absDir); err == nil && fi.IsDir() {
			m.http.MountAdminStatic(p.ID, absDir)
		} else {
			// 目录不存在不致命；如果你想严格要求，可以这里返回错误
		}
	}

	// 2) process 模式：起子进程 → 健康检查 → 挂 API 反代
	if p.Runtime.Kind == plugin_mgr.RuntimeKindProcess {
		if m.sup == nil {
			return plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("enable"), plugin_mgr.WithMsg("supervisor not initialized"))
		}
		// 起
		h, err := m.sup.Start(ctx, p.ID, p.Paths.Entry, p.Runtime.Args, p.Runtime.Env, 0)
		if err != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeProcessStartFailed, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
		}
		// 探活（runtime.health.http 为空则立即通过）
		if err := m.waitHealthy(ctx, h.BaseURL, p.Runtime.Health.HTTPPath, p.Runtime.Health.Interval, p.Runtime.Health.Timeout); err != nil {
			_ = m.sup.Stop(ctx, p.ID)
			return plugin_mgr.Wrap(plugin_mgr.CodeHealthcheckFailed, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
		}
		// 反代
		u, _ := url.Parse(h.BaseURL)
		m.http.MountAPIProxy(p.ID, u, p.Endpoints.HTTPBasePath) // 注意：router 里不会再用 basePath 叠加路径
	}

	// 3) 状态落盘
	if err := m.opts.Registry.UpdateState(ctx, p.ID, p.Version, plugin_mgr.StateEnabled); err != nil {
		return err
	}
	return m.opts.Registry.Save(ctx)
}

func (m *managerImpl) Disable(ctx context.Context, id string) error {
	p, err := m.mustGet(ctx, id)
	if err != nil {
		return err
	}
	if m.http == nil {
		return plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("disable"), plugin_mgr.WithMsg("dynamic router not initialized"))
	}

	// 卸载路由
	m.http.Unmount(p.ID)

	// 停止子进程（如有）
	if p.Runtime.Kind == plugin_mgr.RuntimeKindProcess && m.sup != nil {
		_ = m.sup.Stop(ctx, p.ID)
	}

	if err := m.opts.Registry.UpdateState(ctx, p.ID, p.Version, plugin_mgr.StateDisabled); err != nil {
		return err
	}
	return m.opts.Registry.Save(ctx)
}

// 小工具：Get 带错误码
func (m *managerImpl) mustGet(ctx context.Context, id string) (plugin_mgr.Plugin, error) {
	p, ok := m.opts.Registry.Get(ctx, id)
	if !ok {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("get"), plugin_mgr.WithPlugin(id))
	}
	return p, nil
}
