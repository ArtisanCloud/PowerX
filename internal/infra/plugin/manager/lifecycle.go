package manager

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// Enable: 启用插件 = (挂 Admin 静态) + (启动进程并健康检查) + (挂 API 反代) + (更新注册表)
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
		}
	}

	// 2) process 模式：起子进程 → 健康检查 → 挂 API 反代
	if p.Runtime.Kind == plugin_mgr.RuntimeKindProcess {
		if m.sup == nil {
			return plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("enable"), plugin_mgr.WithMsg("supervisor not initialized"))
		}

		// 从 manifest.Health 组装 supervisor 选项
		h := p.Runtime.Health
		healthPath := firstNonEmpty(h.HTTPPath, "/healthz")
		supOpts := supervisor.Options{
			HealthPath:     healthPath,
			HealthInterval: parseDurDefault(h.Interval, 2*time.Second),
			HealthTimeout:  parseDurDefault(h.Timeout, 1*time.Second),
			AutoRestart:    true,
			BackoffBase:    time.Second,
			BackoffMax:     10 * time.Second,
		}

		// 启动子进程（自动分配端口，PORT 注入）
		envMap := p.Runtime.Env
		port, err := m.sup.Start(ctx, p.ID, p.Paths.Entry, p.Runtime.Args, envMap, supOpts)
		if err != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeProcessStartFailed, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
		}
		baseURL := "http://127.0.0.1:" + strconv.Itoa(port)

		// 健康探活（等待到 healthy 再对外挂载）
		if err := waitHealthy(ctx, baseURL, healthPath, supOpts.HealthInterval, supOpts.HealthTimeout); err != nil {
			_ = m.sup.Stop(p.ID)
			return plugin_mgr.Wrap(plugin_mgr.CodeHealthcheckFailed, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
		}

		// 反向代理挂载
		u, _ := url.Parse(baseURL)
		basePath := p.Endpoints.HTTPBasePath
		if basePath == "" {
			basePath = "/"
		}
		m.http.MountAPIProxy(p.ID, u, basePath)
	}

	// 3) 状态落盘
	if err := m.opts.Registry.UpdateState(ctx, p.ID, p.Version, plugin_mgr.StateEnabled); err != nil {
		return err
	}
	err = m.opts.Registry.Save(ctx)
	if err != nil {
		return err
	}

	if m.opts.PostEnable != nil {
		// 从 ctx 或上层传参拿 tenantID；没有就用 0（系统）
		tid := reqctx.GetTenantID(ctx) // 你已有 reqctx.GetTenantID 或外层注入
		if err := m.opts.PostEnable(ctx, tid, p.ID); err != nil {
			return err
		}
	}
	return nil
}

// Disable: 停用插件 = (卸载路由) + (停进程) + (更新注册表)
func (m *managerImpl) Disable(ctx context.Context, id string) error {
	p, err := m.mustGet(ctx, id)
	if err != nil {
		return err
	}
	if m.http == nil {
		return plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("disable"), plugin_mgr.WithMsg("dynamic router not initialized"))
	}

	// 卸载路由（Admin+API 一键摘掉）
	m.http.Unmount(p.ID)

	// 停止子进程（如有）
	if p.Runtime.Kind == plugin_mgr.RuntimeKindProcess && m.sup != nil {
		_ = m.sup.Stop(p.ID) // 注意：新版 Stop(id string) 无 ctx
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

// ======== 辅助函数 ========

func firstNonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}

// Interval/Timeout 在 manifest 里是 string（"2s"），这里解析；解析失败用默认值
func parseDurDefault(s string, d time.Duration) time.Duration {
	if strings.TrimSpace(s) == "" {
		return d
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return d
	}
	return v
}

// 等待健康：直到返回 2xx 或超时/ctx 结束
func waitHealthy(ctx context.Context, baseURL, healthPath string, interval, timeout time.Duration) error {
	if strings.TrimSpace(healthPath) == "" {
		return nil
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}
	url := strings.TrimRight(baseURL, "/") + healthPath

	// 总等待上限：优先用 ctx.Deadline，否则给个保底 30s
	deadline := time.Now().Add(30 * time.Second)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}

	cli := &http.Client{Timeout: timeout}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := cli.Do(req)
		if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("health check canceled: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("health check timeout for %s", url)
}

// 将 ["KEY=VAL","FOO=BAR"] 转为 map[string]string，非法项跳过
func envListToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		if e == "" {
			continue
		}
		if i := strings.IndexByte(e, '='); i > 0 {
			k := strings.TrimSpace(e[:i])
			v := e[i+1:]
			if k != "" {
				m[k] = v
			}
		}
	}
	return m
}
