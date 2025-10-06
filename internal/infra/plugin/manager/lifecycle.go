// internal/infra/plugin/manager/lifecycle.go
package manager

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// Enable: 启用插件 = (挂 Admin 静态) + (启动进程并健康检查) + (挂 API/Admin 反代) + (更新注册表)
// internal/infra/plugin/manager/lifecycle.go
func (m *managerImpl) Enable(ctx context.Context, id string) error {
	p, err := m.mustGet(ctx, id)

	if err != nil {
		return err
	}
	log.Printf("[plugin-enable] id=%s ver=%s state=%s menus.top=%d admin=%d",
		p.ID, p.Version, p.State, len(p.Menus), len(p.Frontend.Admin.Menus))

	if m.http == nil {
		return plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("enable"), plugin_mgr.WithMsg("dynamic router not initialized"))
	}

	InstallPolicy(m.http, p.ID, PolicyFromPlugin(p))

	// ---------- Admin 静态兜底（保留，不影响进程模式） ----------
	if p.Frontend.Admin.Kind == plugin_mgr.FrontendKindStatic && p.Paths.FrontendAdminDir != "" {
		if abs, err := filepath.Abs(p.Paths.FrontendAdminDir); err == nil {
			if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
				m.http.MountAdminStatic(p.ID, abs)
			}
		}
	}

	// ---------- 后端：进程模式 ----------
	if p.Runtime.Kind != plugin_mgr.RuntimeKindProcess {
		return plugin_mgr.NewError(
			plugin_mgr.CodeUnsupportedRuntime,
			plugin_mgr.WithOp("enable"),
			plugin_mgr.WithMsg("only process runtime is supported"),
			plugin_mgr.WithPlugin(id),
		)
	}

	// supervisor 未初始化（保持 Internal）
	if m.sup == nil {
		return plugin_mgr.NewError(
			plugin_mgr.CodeInternal,
			plugin_mgr.WithOp("enable"),
			plugin_mgr.WithMsg("supervisor not initialized"),
		)
	}

	// 前端 process 形态但缺少必要 entry 时（原来若用了 CodeInvalid）
	if p.Frontend.Admin.Kind == plugin_mgr.FrontendKindProcess &&
		(p.Frontend.Admin.Process == nil || strings.TrimSpace(p.Frontend.Admin.Process.Entry) == "") {
		return plugin_mgr.NewError(
			plugin_mgr.CodeInvalidArg,
			plugin_mgr.WithOp("enable"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithMsg("frontend.admin.process.entry is required when kind=process"),
		)
	}

	apiHC := p.Runtime.Health
	apiHealthPath := utils.FirstNonEmpty(apiHC.HTTPPath, "/healthz")
	supOpts := supervisor.Options{
		HealthPath:     apiHealthPath,
		HealthInterval: parseDurDefault(apiHC.Interval, 2*time.Second),
		HealthTimeout:  parseDurDefault(apiHC.Timeout, 1*time.Second),
		AutoRestart:    true,
		BackoffBase:    time.Second,
		BackoffMax:     10 * time.Second,
	}

	// 后端 env
	envAPI := cloneEnvMap(p.Runtime.Env)
	for k, v := range m.hostEnvForPlugin(p) {
		envAPI[k] = v
	}
	envAPI["POWERX_PLUGIN_ID"] = p.ID
	envAPI["POWERX_PLUGIN_VERSION"] = p.Version
	if p.Paths.Root != "" {
		envAPI["POWERX_PLUGIN_ROOT"] = p.Paths.Root
	}
	if p.Paths.ConfigDir != "" {
		envAPI["POWERX_PLUGIN_CONFIG_DIR"] = p.Paths.ConfigDir
	}
	if p.Paths.HostValuesFile != "" {
		envAPI["POWERX_PLUGIN_HOST_VALUES"] = p.Paths.HostValuesFile
	}
	envAPI["POWERX_PROXY"] = "1"
	m.injectGatewaySecurityEnv(envAPI, p.ID)

	// 内部令牌
	internalToken := os.Getenv("POWERX_INTERNAL_TOKEN")
	if internalToken == "" {
		internalToken = utils.RandomString(48)
	}
	envAPI["POWERX_INTERNAL_TOKEN"] = internalToken
	m.mu.Lock()
	if m.tokens == nil {
		m.tokens = map[string]string{}
	}
	m.tokens[p.ID] = internalToken
	m.mu.Unlock()

	// —— HTTP 绑定（由 supervisor 动态分配端口）
	if strings.TrimSpace(envAPI["POWERX_HTTP_ADDR"]) == "" {
		envAPI["POWERX_HTTP_ADDR"] = supervisor.DynamicBindPlaceholder
	}

	// —— gRPC 绑定（如需）
	grpcAddr := strings.TrimSpace(envAPI["POWERX_GRPC_ADDR"])
	if grpcAddr == "" && p.HostConfig != nil && p.HostConfig.Values != nil {
		grpcAddr = strings.TrimSpace(p.HostConfig.Values["POWERX_GRPC_ADDR"])
	}
	if grpcAddr == "" {
		if gp, err := pickFreePortLocal(); err == nil {
			grpcAddr = fmt.Sprintf("127.0.0.1:%d", gp)
		} else {
			grpcAddr = "127.0.0.1:0"
		}
	}
	envAPI["POWERX_GRPC_ADDR"] = grpcAddr
	if i := strings.LastIndexByte(grpcAddr, ':'); i >= 0 && i+1 < len(grpcAddr) {
		envAPI["POWERX_GRPC_PORT"] = grpcAddr[i+1:]
	}

	// —— 启动后端
	apiPort, err := m.sup.Start(ctx, p.ID, p.Paths.Entry, p.Runtime.Args, envAPI, supOpts)
	if err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeProcessStartFailed, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
	}
	apiBaseURL := "http://127.0.0.1:" + strconv.Itoa(apiPort)

	// 健康
	if err := waitHealthy(ctx, apiBaseURL, apiHealthPath, supOpts.HealthInterval, supOpts.HealthTimeout); err != nil {
		_ = m.sup.Stop(p.ID)
		return plugin_mgr.Wrap(plugin_mgr.CodeHealthcheckFailed, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
	}

	// —— 挂 API 反代
	basePath := p.Endpoints.HTTPBasePath
	if basePath == "" {
		basePath = "/"
	}
	apiURL, _ := url.Parse(apiBaseURL)
	m.http.MountAPIProxy(p.ID, apiURL, basePath, apiHealthPath)

	// ---------- 前端：三种形态 ----------
	switch p.Frontend.Admin.Kind {
	case plugin_mgr.FrontendKindStatic:
		// 已挂兜底，啥也不做

	case plugin_mgr.FrontendKindProxy:
		// 前端由后端提供（同端口），直接代理到后端
		m.http.MountAdminProxy(p.ID, apiURL)

	case plugin_mgr.FrontendKindProcess:
		// —— 只按插件清单执行 entry/args，不做任何硬编码
		adm := p.Frontend.Admin.Process
		if adm == nil || strings.TrimSpace(adm.Entry) == "" {
			// 没给进程就回退：若有静态目录挂静态，否则复用后端端口
			if p.Paths.FrontendAdminDir != "" {
				if abs, err := filepath.Abs(p.Paths.FrontendAdminDir); err == nil {
					if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
						m.http.MountAdminStatic(p.ID, abs)
					}
				}
			} else {
				m.http.MountAdminProxy(p.ID, apiURL)
			}
			break
		}

		adminEntry := strings.TrimSpace(adm.Entry)
		adminArgs := append([]string{}, adm.Args...)

		// 把「像路径」的判断写成函数
		isPathLike := func(s string) bool {
			s = strings.TrimSpace(s)
			if s == "" {
				return false
			}
			// . 开头 / 绝对 / 包含分隔符（兼容 Windows 的反斜杠）
			return strings.HasPrefix(s, ".") ||
				strings.HasPrefix(s, "/") ||
				strings.Contains(s, "/") ||
				strings.Contains(s, `\`)
		}

		// entry：像路径才转绝对；裸命令（node、bun 等）保持原样交给 PATH
		if isPathLike(adminEntry) && !filepath.IsAbs(adminEntry) {
			adminEntry = filepath.Join(p.Paths.Root, adminEntry)
		}

		// args[0]：如果是脚本/可执行的相对路径，也要转绝对（相对插件根）
		if len(adminArgs) > 0 && isPathLike(adminArgs[0]) && !filepath.IsAbs(adminArgs[0]) {
			adminArgs[0] = filepath.Join(p.Paths.Root, adminArgs[0])
		}

		// 进程环境：只做合并与标识，不改你的变量名和值
		envADM := cloneEnvMap(adm.Env)
		for k, v := range m.hostEnvForPlugin(p) {
			envADM[k] = v
		}
		envADM["NODE_ENV"] = "production"
		envADM["POWERX_PROXY"] = "1"
		envADM["POWERX_ADMIN_BASE"] = fmt.Sprintf("/_p/%s/admin/", p.ID)

		if _, ok := envADM["NITRO_HOST"]; !ok {
			envADM["NITRO_HOST"] = "127.0.0.1" // 无害缺省，保留可被 env 覆盖
		}
		// 让 supervisor 分配 HTTP 端口（如需要）
		if strings.TrimSpace(envADM["POWERX_HTTP_ADDR"]) == "" {
			envADM["POWERX_HTTP_ADDR"] = supervisor.DynamicBindPlaceholder
		}

		// 健康探针参数（按清单）
		adminHC := adm.Health
		adminHealthPath := utils.FirstNonEmpty(adminHC.HTTPPath, "/healthz")
		adminSup := supervisor.Options{
			HealthPath:     adminHealthPath,
			HealthInterval: parseDurDefault(adminHC.Interval, 2*time.Second),
			HealthTimeout:  parseDurDefault(adminHC.Timeout, 1*time.Second),
			AutoRestart:    true,
			BackoffBase:    time.Second,
			BackoffMax:     10 * time.Second,
		}

		adminProcID := p.ID + "_admin"
		// ★ 关键：按插件给的 entry/args 原样执行（entry 仅做绝对路径解析）
		adminPort, err := m.sup.Start(ctx, adminProcID, adminEntry, adminArgs, envADM, adminSup)
		if err != nil {
			fmt.Printf("[plugin-enable] plugin=%s admin process start failed: %v (fallback)\n", p.ID, err)
			// 回退：若有静态目录挂静态，否则复用后端端口
			if p.Paths.FrontendAdminDir != "" {
				if abs, err := filepath.Abs(p.Paths.FrontendAdminDir); err == nil {
					if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
						m.http.MountAdminStatic(p.ID, abs)
					}
				}
			} else {
				m.http.MountAdminProxy(p.ID, apiURL)
			}
			break
		}

		adminBaseURL := "http://127.0.0.1:" + strconv.Itoa(adminPort)
		// 健康探测失败也不阻塞；最多少等一会
		_ = waitHealthy(ctx, adminBaseURL, adminHealthPath, adminSup.HealthInterval, adminSup.HealthTimeout)

		// ★ 成功：挂 Admin 反代
		adminURL, _ := url.Parse(adminBaseURL)
		m.http.MountAdminProxy(p.ID, adminURL)
		fmt.Printf("[plugin-enable] plugin=%s admin_upstream=%s entry=%q args=%v\n",
			p.ID, adminBaseURL, adminEntry, adminArgs)

	}

	// ---------- 状态 ----------
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
		return plugin_mgr.NewError(
			plugin_mgr.CodeInternal,
			plugin_mgr.WithOp("disable"),
			plugin_mgr.WithMsg("dynamic router not initialized"),
		)
	}

	// 卸载路由（Admin+API）
	m.http.Unmount(p.ID)

	// 停止子进程（如有）
	if p.Runtime.Kind == plugin_mgr.RuntimeKindProcess && m.sup != nil {
		_ = m.sup.Stop(p.ID)
		_ = m.sup.Stop(p.ID + "_admin") // ← 停掉 admin 进程（如果有）
	}

	if err := m.opts.Registry.UpdateState(ctx, p.ID, p.Version, plugin_mgr.StateDisabled); err != nil {
		return err
	}
	return m.opts.Registry.Save(ctx)
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
	healthURL := strings.TrimRight(baseURL, "/") + healthPath
	deadline := time.Now().Add(30 * time.Second)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}
	cli := &http.Client{Timeout: timeout}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
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
	return fmt.Errorf("health check timeout for %s", healthURL)
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

func (m *managerImpl) injectGatewaySecurityEnv(env map[string]string, pluginID string) {
	if env == nil {
		return
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return
	}

	env["POWERX_SECURITY_MODE"] = "jwt"
	env["POWERX_SECURITY_JWT_AUDIENCE"] = "plugin:" + pluginID
	env["POWERX_SECURITY_JWT_SCOPE"] = "access"

	if cfg := m.opts.CoreConfig; cfg != nil {
		if secret := strings.TrimSpace(cfg.Auth.JWTSecret); secret != "" {
			env["POWERX_SECURITY_JWT_SECRET"] = secret
			env["POWERX_SECURITY_CTX_HMAC_SECRET"] = secret
		}
		if issuer := strings.TrimSpace(cfg.Auth.Issuer); issuer != "" {
			env["POWERX_SECURITY_JWT_ISSUER"] = issuer
		}
	}

	if secret := strings.TrimSpace(env["POWERX_SECURITY_JWT_SECRET"]); secret != "" {
		if cur := strings.TrimSpace(env["POWERX_SECURITY_CTX_HMAC_SECRET"]); cur == "" {
			env["POWERX_SECURITY_CTX_HMAC_SECRET"] = secret
		}
	}
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
