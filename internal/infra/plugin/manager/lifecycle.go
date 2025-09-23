package manager

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
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

	InstallPolicy(m.http, p.ID, PolicyFromPlugin(p))

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

		// —— 从 manifest.Health 组装 supervisor 选项
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

		// —— 进程环境：基础 env + 宿主注入 env
		envMap := cloneEnvMap(p.Runtime.Env)
		hostEnv := m.hostEnvForPlugin(p) // 若你已有同名方法，可直接用你的实现
		for k, v := range hostEnv {
			envMap[k] = v
		}
		if p.Paths.Root != "" {
			envMap["POWERX_PLUGIN_ROOT"] = p.Paths.Root
		}
		if p.Paths.ConfigDir != "" {
			envMap["POWERX_PLUGIN_CONFIG_DIR"] = p.Paths.ConfigDir
		}
		if p.Paths.HostValuesFile != "" {
			envMap["POWERX_PLUGIN_HOST_VALUES"] = p.Paths.HostValuesFile
		}
		envMap["POWERX_PLUGIN_VERSION"] = p.Version

		// —— 内部通信令牌
		internalToken := os.Getenv("POWERX_INTERNAL_TOKEN")
		if internalToken == "" {
			internalToken = utils.RandomString(48)
		}
		envMap["POWERX_INTERNAL_TOKEN"] = internalToken
		envMap["POWERX_PLUGIN_ID"] = p.ID
		m.mu.Lock()
		if m.tokens == nil {
			m.tokens = map[string]string{}
		}
		m.tokens[p.ID] = internalToken
		m.mu.Unlock()

		// —— HTTP 绑定（由 Supervisor 动态分配 PORT + PX_BIND_ADDR）
		runtimeBind := strings.TrimSpace(p.Runtime.Env["PX_BIND_ADDR"])
		hostBind := ""
		if hc := p.HostConfig; hc != nil && hc.Values != nil {
			hostBind = strings.TrimSpace(hc.Values["PX_BIND_ADDR"])
		}
		dynamicHTTP := hostBind == ""
		if dynamicHTTP {
			// 用占位符告诉 Supervisor：请你挑一个可用端口并注入 PORT/PX_BIND_ADDR
			envMap["PX_BIND_ADDR"] = supervisor.DynamicBindPlaceholder
		}

		// —— gRPC 绑定（新增：由 Manager 统一分配）
		grpcAddr := strings.TrimSpace(p.Runtime.Env["PX_GRPC_ADDR"])
		if grpcAddr == "" && p.HostConfig != nil && p.HostConfig.Values != nil {
			grpcAddr = strings.TrimSpace(p.HostConfig.Values["PX_GRPC_ADDR"])
		}
		if grpcAddr == "" || grpcAddr == ":9101" || grpcAddr == "127.0.0.1:9101" {
			// 没配或是老的固定 9101：为它挑一个空闲端口
			gp, err := pickFreePortLocal()
			if err == nil {
				grpcAddr = fmt.Sprintf("127.0.0.1:%d", gp)
			} else {
				// 实在拿不到，就给个 0（让插件自己挑）；建议尽快改插件读取 PX_GRPC_ADDR
				grpcAddr = "127.0.0.1:0"
			}
		}
		envMap["PX_GRPC_ADDR"] = grpcAddr
		// 兼容一些旧插件读取 GRPC_ADDR/GRPC_PORT
		envMap["GRPC_ADDR"] = grpcAddr
		if i := strings.LastIndexByte(grpcAddr, ':'); i >= 0 && i+1 < len(grpcAddr) {
			envMap["PX_GRPC_PORT"] = grpcAddr[i+1:]
			envMap["GRPC_PORT"] = grpcAddr[i+1:]
		}

		fmt.Printf("[plugin-enable] plugin=%s runtime_http=%q host_http=%q http_effective=%q dynamic_http=%v\n",
			p.ID, runtimeBind, hostBind, strings.TrimSpace(envMap["PX_BIND_ADDR"]), dynamicHTTP)
		fmt.Printf("[plugin-enable] plugin=%s grpc_bind=%q\n", p.ID, grpcAddr)

		// —— 启动子进程（Supervisor 会再分配 HTTP 端口，注入 PORT）
		port, err := m.sup.Start(ctx, p.ID, p.Paths.Entry, p.Runtime.Args, envMap, supOpts)
		if err != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeProcessStartFailed, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
		}
		baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
		fmt.Printf("[plugin-enable] plugin=%s assigned_http_port=%d http_bind_effective=%q\n",
			p.ID, port, strings.TrimSpace(envMap["PX_BIND_ADDR"]))

		// —— 健康探活（OK 再挂反代）
		if err := waitHealthy(ctx, baseURL, healthPath, supOpts.HealthInterval, supOpts.HealthTimeout); err != nil {
			_ = m.sup.Stop(p.ID)
			return plugin_mgr.Wrap(plugin_mgr.CodeHealthcheckFailed, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
		}

		// —— 反向代理挂载
		u, _ := url.Parse(baseURL)
		basePath := p.Endpoints.HTTPBasePath
		if basePath == "" {
			basePath = "/"
		}
		m.http.MountAPIProxy(p.ID, u, basePath, healthPath)
	}

	// 3) 状态落盘
	if err := m.opts.Registry.UpdateState(ctx, p.ID, p.Version, plugin_mgr.StateEnabled); err != nil {
		return err
	}
	if err := m.opts.Registry.Save(ctx); err != nil {
		return err
	}

	if m.opts.PostEnable != nil {
		if tid := reqctx.GetTenantID(ctx); tid > 0 {
			if err := m.opts.PostEnable(ctx, tid, p.ID); err != nil {
				return err
			}
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

	// 卸载路由（Admin+API）
	m.http.Unmount(p.ID)

	// 停止子进程（如有）
	if p.Runtime.Kind == plugin_mgr.RuntimeKindProcess && m.sup != nil {
		_ = m.sup.Stop(p.ID)
	}

	if err := m.opts.Registry.UpdateState(ctx, p.ID, p.Version, plugin_mgr.StateDisabled); err != nil {
		return err
	}
	return m.opts.Registry.Save(ctx)
}

// —— 工具/兼容 ——
func firstNonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}

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

func waitHealthy(ctx context.Context, baseURL, healthPath string, interval, timeout time.Duration) error {
	if strings.TrimSpace(healthPath) == "" {
		return nil
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}
	url := strings.TrimRight(baseURL, "/") + healthPath
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

func cloneEnvMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

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

func pickFreePortLocal() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Get 带错误码
func (m *managerImpl) mustGet(ctx context.Context, id string) (plugin_mgr.Plugin, error) {
	p, ok := m.opts.Registry.Get(ctx, id)
	if !ok {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("get"), plugin_mgr.WithPlugin(id))
	}
	return p, nil
}
