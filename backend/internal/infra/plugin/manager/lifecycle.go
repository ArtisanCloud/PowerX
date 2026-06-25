// internal/infra/plugin/manager/lifecycle.go
package manager

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// Enable: 启用插件 = (挂 Admin 静态) + (启动进程并健康检查) + (挂 API/Admin 反代) + (更新注册表)
// internal/infra/plugin/manager/lifecycle.go
func (m *managerImpl) Enable(ctx context.Context, id string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// 进程生命周期不能绑定在 HTTP 请求上下文上：
	// 安装/启用 API 返回后请求 ctx 会被取消，若直接传给 exec.CommandContext
	// 将导致刚拉起的插件进程被意外杀掉（表现为安装后首访 502，重启后恢复）。
	procCtx := context.WithoutCancel(ctx)
	p, err := m.mustGet(ctx, id)

	if err != nil {
		return err
	}
	runtimeCred, err := m.resolvePluginRuntimeCredential(ctx, p.ID)
	if err != nil {
		return plugin_mgr.NewError(
			plugin_mgr.CodeLifecycleError,
			plugin_mgr.WithOp("enable"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(p.Version),
			plugin_mgr.WithMsg("GW_BOOTSTRAP_CONTRACT_BROKEN: %v", err),
		)
	}
	if err := m.ensureDelegatedHostContractForEnable(&p, runtimeCred); err != nil {
		logger.WarnF(ctx, "[plugin-enable] id=%s host contract auto-repair failed: %v", p.ID, err)
	}
	logger.InfoF(ctx, "[plugin-enable] id=%s ver=%s state=%s admin_menus=%d",
		p.ID, p.Version, p.State, len(p.Frontend.Admin.Menus))

	if m.http == nil {
		return plugin_mgr.NewError(plugin_mgr.CodeInternal, plugin_mgr.WithOp("enable"), plugin_mgr.WithMsg("dynamic router not initialized"))
	}

	InstallPolicy(m.http, p.ID, PolicyFromPlugin(p))

	var (
		backendInfo supervisor.ProcInfo
		hasBackend  bool
		hasAdmin    bool
	)
	if m.sup != nil {
		if info, ok := m.sup.Status(p.ID); ok {
			backendInfo = info
			hasBackend = true
		}
		if _, ok := m.sup.Status(p.ID + "_admin"); ok {
			hasAdmin = true
		}
	}

	backendAlive := hasBackend &&
		backendInfo.State != supervisor.ProcStopped &&
		backendInfo.State != supervisor.ProcExited &&
		backendInfo.State != supervisor.ProcCrashed

	if p.State == plugin_mgr.StateEnabled && backendAlive {
		return plugin_mgr.NewError(
			plugin_mgr.CodeAlreadyEnabled,
			plugin_mgr.WithOp("enable"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(p.Version),
		)
	}

	// 清理遗留进程记录，避免 Start 时命中 “already running”
	if m.sup != nil {
		if hasBackend {
			_ = m.sup.Stop(p.ID)
		}
		if hasAdmin {
			_ = m.sup.Stop(p.ID + "_admin")
		}
	}

	// 重新启用前先卸载历史路由，避免重复挂载
	if m.http != nil {
		m.http.Unmount(p.ID)
	}
	// ---------- Admin 静态目录 ----------
	// process 类型的 admin 也可能通过 public/ 暴露 icon、favicon 等资产。
	// 这些资产需要在插件启用前后都能被 /_p/{id}/admin/* 稳定访问。
	if p.Paths.FrontendAdminDir != "" {
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
	legacyBackendHealthPath := ""
	if p.Backend != nil {
		legacyBackendHealthPath = strings.TrimSpace(p.Backend.Health)
	}
	apiHealthPath := resolveHealthPath(apiHC.HTTPPath, legacyBackendHealthPath, p.Endpoints.HTTPBasePath)
	supOpts := supervisor.Options{
		HealthPath:     apiHealthPath,
		HealthInterval: parseDurDefault(apiHC.Interval, 2*time.Second),
		HealthTimeout:  parseDurDefault(apiHC.Timeout, 1*time.Second),
		AutoRestart:    false,
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
	// 宿主托管插件始终按 delegated_proxy 运行，不因启动时是否有租户上下文切换 runtime mode。
	envAPI["POWERX_PROXY"] = "1"
	applyDelegatedRuntimeEnv(envAPI)
	logger.InfoF(ctx, "[plugin-enable] plugin=%s delegated_runtime_env mode=%s mode_legacy=%s proxy=%s auth_scheme=%s",
		p.ID,
		strings.TrimSpace(envAPI["IAMMode"]),
		strings.TrimSpace(envAPI["IAM_MODE"]),
		strings.TrimSpace(envAPI["POWERX_PROXY"]),
		strings.TrimSpace(envAPI["PX_GATEWAY_AUTH_SCHEME"]),
	)
	if err := m.injectGlobalGatewayRuntimeEnv(envAPI, p.ID, runtimeCred); err != nil {
		recordGatewayContractValid(p.ID, false)
		recordGatewayContractProbeResult("bootstrap_failed")
		emitGatewayContractAudit(ctx, p.ID, map[string]any{
			"gateway_base_url_present": strings.TrimSpace(envAPI["PX_GATEWAY_BASE_URL"]) != "",
			"auth_scheme":              strings.TrimSpace(envAPI["PX_GATEWAY_AUTH_SCHEME"]),
			"reason":                   err.Error(),
		}, "bootstrap_failed")
		return plugin_mgr.NewError(
			plugin_mgr.CodeLifecycleError,
			plugin_mgr.WithOp("enable"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(p.Version),
			plugin_mgr.WithMsg("GW_BOOTSTRAP_CONTRACT_BROKEN: %v", err),
		)
	}

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

	if secret := strings.TrimSpace(envAPI["POWERX_SECURITY_CTX_HMAC_SECRET"]); secret != "" {
		if strings.TrimSpace(envAPI["PLUGIN_CTX_HMAC_SECRET"]) == "" {
			envAPI["PLUGIN_CTX_HMAC_SECRET"] = secret
		}
	}
	if issuer := strings.TrimSpace(envAPI["POWERX_SECURITY_JWT_ISSUER"]); issuer != "" {
		if strings.TrimSpace(envAPI["POWERX_CTX_ISSUER"]) == "" {
			envAPI["POWERX_CTX_ISSUER"] = issuer
		}
	}
	if audience := strings.TrimSpace(envAPI["POWERX_SECURITY_JWT_AUDIENCE"]); audience != "" {
		if strings.TrimSpace(envAPI["POWERX_CTX_AUDIENCE"]) == "" {
			envAPI["POWERX_CTX_AUDIENCE"] = audience
		}
	}
	if strings.TrimSpace(envAPI["POWERX_CTX_TTL"]) == "" {
		if cfg := m.opts.CoreConfig; cfg != nil {
			if ttl := strings.TrimSpace(cfg.Auth.AccessTTLStr); ttl != "" {
				envAPI["POWERX_CTX_TTL"] = ttl
			}
		}
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
	logPluginGatewayLaunchEnv(ctx, p.ID, envAPI)
	apiPort, err := m.sup.Start(procCtx, p.ID, p.Paths.Entry, p.Runtime.Args, envAPI, supOpts)
	if err != nil {
		// 若启动失败，避免保留半初始化的进程记录
		_ = m.sup.Stop(p.ID)
		return plugin_mgr.Wrap(plugin_mgr.CodeProcessStartFailed, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
	}
	apiBaseURL := "http://127.0.0.1:" + strconv.Itoa(apiPort)

	// 健康
	if err := waitHealthy(ctx, apiBaseURL, apiHealthPath, supOpts.HealthInterval, supOpts.HealthTimeout); err != nil {
		_ = m.sup.Stop(p.ID)
		return plugin_mgr.Wrap(plugin_mgr.CodeHealthcheckFailed, err, plugin_mgr.WithOp("enable"), plugin_mgr.WithPlugin(id))
	}
	if err := m.probeGatewayContract(ctx, p.ID, apiBaseURL, apiHealthPath, envAPI, false, ""); err != nil {
		recordGatewayContractValid(p.ID, false)
		recordGatewayContractProbeResult("probe_failed")
		emitGatewayContractAudit(ctx, p.ID, map[string]any{
			"gateway_base_url_present": strings.TrimSpace(envAPI["PX_GATEWAY_BASE_URL"]) != "",
			"auth_scheme":              strings.TrimSpace(envAPI["PX_GATEWAY_AUTH_SCHEME"]),
			"tenant_uuid_present":      false,
			"reason":                   err.Error(),
		}, "probe_failed")
		_ = m.sup.Stop(p.ID)
		_ = m.sup.Stop(p.ID + "_admin")
		m.http.Unmount(p.ID)
		return plugin_mgr.NewError(
			plugin_mgr.CodeLifecycleError,
			plugin_mgr.WithOp("enable"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(p.Version),
			plugin_mgr.WithMsg("enable_failed_gateway_contract: %v", err),
		)
	}
	recordGatewayContractValid(p.ID, true)
	recordGatewayContractProbeResult("success")
	emitGatewayContractAudit(ctx, p.ID, map[string]any{
		"gateway_base_url_present": strings.TrimSpace(envAPI["PX_GATEWAY_BASE_URL"]) != "",
		"auth_scheme":              strings.TrimSpace(envAPI["PX_GATEWAY_AUTH_SCHEME"]),
		"tenant_uuid_present":      false,
	}, "probe_success")

	// —— 挂 API 反代
	basePath := p.Endpoints.HTTPBasePath
	if basePath == "" {
		basePath = "/"
	}
	apiURL, _ := url.Parse(apiBaseURL)
	m.http.MountAPIProxy(p.ID, apiURL, basePath, apiHealthPath)
	logger.InfoF(ctx, "[plugin-enable] plugin=%s api_upstream=%s base_path=%s health=%s",
		p.ID, apiBaseURL, basePath, apiHealthPath)

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
		if secret := strings.TrimSpace(envADM["POWERX_SECURITY_CTX_HMAC_SECRET"]); secret != "" {
			if strings.TrimSpace(envADM["PLUGIN_CTX_HMAC_SECRET"]) == "" {
				envADM["PLUGIN_CTX_HMAC_SECRET"] = secret
			}
		}
		if issuer := strings.TrimSpace(envADM["POWERX_SECURITY_JWT_ISSUER"]); issuer != "" {
			if strings.TrimSpace(envADM["POWERX_CTX_ISSUER"]) == "" {
				envADM["POWERX_CTX_ISSUER"] = issuer
			}
		}
		if audience := strings.TrimSpace(envADM["POWERX_SECURITY_JWT_AUDIENCE"]); audience != "" {
			if strings.TrimSpace(envADM["POWERX_CTX_AUDIENCE"]) == "" {
				envADM["POWERX_CTX_AUDIENCE"] = audience
			}
		}
		if strings.TrimSpace(envADM["POWERX_CTX_TTL"]) == "" {
			if cfg := m.opts.CoreConfig; cfg != nil {
				if ttl := strings.TrimSpace(cfg.Auth.AccessTTLStr); ttl != "" {
					envADM["POWERX_CTX_TTL"] = ttl
				}
			}
		}
		envADM["NODE_ENV"] = "production"
		if err := m.injectGlobalGatewayRuntimeEnv(envADM, p.ID, runtimeCred); err != nil {
			logger.WarnF(ctx, "[plugin-enable] id=%s admin bootstrap gateway env failed: %v", p.ID, err)
		}
		envADM["POWERX_PROXY"] = "1"
		applyDelegatedRuntimeEnv(envADM)
		logger.InfoF(ctx, "[plugin-enable] plugin=%s admin_delegated_runtime_env mode=%s mode_legacy=%s proxy=%s auth_scheme=%s",
			p.ID,
			strings.TrimSpace(envADM["IAMMode"]),
			strings.TrimSpace(envADM["IAM_MODE"]),
			strings.TrimSpace(envADM["POWERX_PROXY"]),
			strings.TrimSpace(envADM["PX_GATEWAY_AUTH_SCHEME"]),
		)
		logger.InfoF(ctx, "[plugin-enable] plugin=%s ws_contract ws_origin=%s ws_path=%s core_base=%s",
			p.ID,
			strings.TrimSpace(envADM["NUXT_PUBLIC_WS_ORIGIN"]),
			strings.TrimSpace(envADM["NUXT_PUBLIC_WS_PATH"]),
			strings.TrimSpace(envADM["NUXT_PUBLIC_POWERX_CORE_BASE"]),
		)
		envADM["POWERX_ADMIN_BASE"] = fmt.Sprintf("/_p/%s/admin/", p.ID)

		if _, ok := envADM["NITRO_HOST"]; !ok {
			envADM["NITRO_HOST"] = "127.0.0.1" // 无害缺省，保留可被 env 覆盖
		}
		// 让 supervisor 分配 HTTP 端口（如需要）
		if strings.TrimSpace(envADM["POWERX_HTTP_ADDR"]) == "" {
			envADM["POWERX_HTTP_ADDR"] = supervisor.DynamicBindPlaceholder
		}

		// 健康探针参数（按清单）
		// Admin 进程默认不强制 health 探针，避免不同框架/构建形态下
		// 未暴露 /healthz 导致被 supervisor 误判并主动 SIGTERM。
		adminHC := adm.Health
		adminHealthPath := strings.TrimSpace(adminHC.HTTPPath)
		adminSup := supervisor.Options{
			HealthPath:     adminHealthPath,
			HealthInterval: parseDurDefault(adminHC.Interval, 2*time.Second),
			HealthTimeout:  parseDurDefault(adminHC.Timeout, 1*time.Second),
			AutoRestart:    false,
			BackoffBase:    time.Second,
			BackoffMax:     10 * time.Second,
		}

		adminProcID := p.ID + "_admin"
		if strings.EqualFold(strings.TrimSpace(adminEntry), "node") {
			nodeBin := strings.TrimSpace(utils.FirstNonEmpty(envADM["NODE_BIN"], os.Getenv("NODE_BIN")))
			if nodeBin != "" {
				if fi, err := os.Stat(nodeBin); err == nil && !fi.IsDir() {
					adminEntry = nodeBin
				} else {
					logger.WarnF(ctx, "[plugin-enable] plugin=%s NODE_BIN invalid: %q err=%v (keep entry=node)", p.ID, nodeBin, err)
				}
			}
		}
		// ★ 关键：按插件给的 entry/args 原样执行（entry 仅做绝对路径解析）
		adminPort, err := m.sup.Start(procCtx, adminProcID, adminEntry, adminArgs, envADM, adminSup)
		if err != nil {
			logger.WarnF(ctx, "[plugin-enable] plugin=%s admin process start failed: %v (fallback)", p.ID, err)
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
		ready := true
		if strings.TrimSpace(adminHealthPath) != "" {
			if err := waitHealthy(ctx, adminBaseURL, adminHealthPath, adminSup.HealthInterval, adminSup.HealthTimeout); err != nil {
				ready = false
				logger.WarnF(ctx, "[plugin-enable] plugin=%s admin health check failed: %v", p.ID, err)
			}
		} else {
			if err := waitSupervisorProcessStable(procCtx, m.sup, adminProcID, 3*time.Second); err != nil {
				ready = false
				logger.WarnF(ctx, "[plugin-enable] plugin=%s admin process not stable after start: %v", p.ID, err)
			}
		}
		if !ready {
			_ = m.sup.Stop(adminProcID)
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

		// ★ 成功：挂 Admin 反代
		adminURL, _ := url.Parse(adminBaseURL)
		m.http.MountAdminProxy(p.ID, adminURL)
		logger.InfoF(ctx, "[plugin-enable] plugin=%s admin_upstream=%s entry=%q args=%v",
			p.ID, adminBaseURL, adminEntry, adminArgs)

	}

	if m.opts.PostEnablePlugin != nil {
		if err := m.opts.PostEnablePlugin(ctx, p, apiBaseURL); err != nil {
			_ = m.sup.Stop(p.ID)
			_ = m.sup.Stop(p.ID + "_admin")
			m.http.Unmount(p.ID)
			return plugin_mgr.Wrap(
				plugin_mgr.CodeLifecycleError,
				err,
				plugin_mgr.WithOp("enable.discover_plugin_skills"),
				plugin_mgr.WithPlugin(p.ID),
				plugin_mgr.WithVersion(p.Version),
			)
		}
	}

	// ---------- 状态 ----------
	if err := m.opts.Registry.UpdateState(ctx, p.ID, p.Version, plugin_mgr.StateEnabled); err != nil {
		return err
	}
	if err := m.opts.Registry.Save(ctx); err != nil {
		return err
	}
	return nil
}

func (m *managerImpl) injectGlobalGatewayRuntimeEnv(env map[string]string, pluginID string, runtimeCred *PluginRuntimeCredential) error {
	if env == nil {
		return fmt.Errorf("gateway contract env is nil")
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return fmt.Errorf("gateway contract plugin id is empty")
	}
	cfg := m.opts.CoreConfig
	if cfg == nil {
		return fmt.Errorf("gateway contract core config missing")
	}
	baseURL := strings.TrimSpace(os.Getenv("POWERX_GATEWAY_BASE_URL"))
	if baseURL == "" {
		port := cfg.Server.Port
		if port <= 0 {
			return fmt.Errorf("GW_CFG_MISSING_BASE_URL: server.port missing")
		}
		baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	env["PX_GATEWAY_BASE_URL"] = strings.TrimRight(baseURL, "/")
	env["PX_GATEWAY_AUTH_SCHEME"] = "bearer"
	applyWSContractEnv(env, cfg)
	deleteDeprecatedGatewayRuntimeEnv(env)
	if err := m.injectRuntimeSTSContract(env, pluginID, runtimeCred); err != nil {
		return err
	}
	return nil
}

func (m *managerImpl) resolvePluginRuntimeCredential(ctx context.Context, pluginID string) (*PluginRuntimeCredential, error) {
	if m == nil || m.opts.RuntimeCredential == nil {
		return nil, fmt.Errorf("GW_CFG_MISSING_STS_PROVIDER: plugin runtime credential provider is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cred, err := m.opts.RuntimeCredential(ctx, pluginID)
	if err != nil {
		return nil, fmt.Errorf("GW_CFG_STS_PROVIDER_FAILED: %w", err)
	}
	if cred == nil {
		return nil, fmt.Errorf("GW_CFG_MISSING_STS_CLIENT: plugin runtime credential is nil")
	}
	return cred, nil
}

func (m *managerImpl) injectRuntimeSTSContract(env map[string]string, pluginID string, cred *PluginRuntimeCredential) error {
	if cred == nil {
		var err error
		cred, err = m.resolvePluginRuntimeCredential(context.Background(), pluginID)
		if err != nil {
			return err
		}
	}
	clientID := strings.TrimSpace(cred.ClientID)
	clientSecret := strings.TrimSpace(cred.ClientSecret)
	tenantUUID := strings.TrimSpace(cred.TenantUUID)
	grpcAddress := strings.TrimSpace(cred.GRPCAddress)
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("GW_CFG_MISSING_STS_CLIENT: delegated host mode requires STS client env")
	}
	if tenantUUID == "" {
		return fmt.Errorf("GW_CFG_MISSING_STS_TENANT: delegated host mode requires POWERX_GRPC_UPSTREAM_TENANT_UUID")
	}
	if grpcAddress == "" {
		return fmt.Errorf("GW_CFG_MISSING_GRPC_UPSTREAM: delegated host mode requires POWERX_GRPC_UPSTREAM_ADDRESS")
	}
	applyRuntimeCredentialToEnv(env, cred)
	return nil
}

func applyRuntimeCredentialToEnv(env map[string]string, cred *PluginRuntimeCredential) {
	if env == nil || cred == nil {
		return
	}
	if baseURL := strings.TrimSpace(cred.GatewayBaseURL); baseURL != "" {
		env["PX_GATEWAY_BASE_URL"] = strings.TrimRight(baseURL, "/")
	}
	env["POWERX_STS_CLIENT_ID"] = strings.TrimSpace(cred.ClientID)
	env["POWERX_STS_CLIENT_SECRET"] = strings.TrimSpace(cred.ClientSecret)
	env["POWERX_STS_AUDIENCE"] = strings.TrimSpace(cred.STSAudience)
	if env["POWERX_STS_AUDIENCE"] == "" {
		env["POWERX_STS_AUDIENCE"] = "powerx:api"
	}
	env["POWERX_STS_SCOPE"] = strings.TrimSpace(cred.STSScope)
	if env["POWERX_STS_SCOPE"] == "" {
		env["POWERX_STS_SCOPE"] = "access"
	}
	env["POWERX_GRPC_UPSTREAM_ADDRESS"] = strings.TrimSpace(cred.GRPCAddress)
	env["POWERX_GRPC_UPSTREAM_TENANT_UUID"] = strings.TrimSpace(cred.TenantUUID)
}

func deleteDeprecatedGatewayRuntimeEnv(env map[string]string) {
	for _, key := range []string{
		"PX_GATEWAY_TENANT_UUID",
		"PX_GATEWAY_API_KEY",
		"PX_PLUGIN_API_KEY",
		"PX_TOOL_TOKEN",
		"PX_PLUGIN_TOOL_TOKEN",
	} {
		delete(env, key)
	}
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

func resolveHealthPath(explicitPath, backendDeclaredPath, httpBasePath string) string {
	explicit := strings.TrimSpace(explicitPath)
	if explicit != "" {
		if !strings.HasPrefix(explicit, "/") {
			return "/" + explicit
		}
		return explicit
	}
	backendPath := strings.TrimSpace(backendDeclaredPath)
	if backendPath != "" {
		if !strings.HasPrefix(backendPath, "/") {
			return "/" + backendPath
		}
		return backendPath
	}
	base := strings.TrimSpace(httpBasePath)
	if base == "" || base == "/" {
		return "/healthz"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = strings.TrimRight(base, "/")
	return base + "/healthz"
}

// applyDelegatedRuntimeEnv injects delegated_proxy runtime contract hints for plugin runtimes.
// Some plugin runtimes read different variable names; keep a small compatibility set.
func applyDelegatedRuntimeEnv(env map[string]string) {
	if env == nil {
		return
	}
	// Enforce delegated IAM for managed plugin runtimes.
	env["POWERX_PROXY"] = "1"
	env["IAMMode"] = "delegated"
	env["IAM_MODE"] = "delegated"
	env["PX_GATEWAY_AUTH_SCHEME"] = "bearer"

	env["TASKBUS_PROVIDER"] = "host"
	env["taskbus_provider"] = "host"
	env["POWERX_TASKBUS_PROVIDER"] = "host"
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

func waitSupervisorProcessStable(ctx context.Context, sup *supervisor.Supervisor, procID string, window time.Duration) error {
	if sup == nil {
		return fmt.Errorf("supervisor is nil")
	}
	if strings.TrimSpace(procID) == "" {
		return fmt.Errorf("proc id is empty")
	}
	if window <= 0 {
		window = 2 * time.Second
	}
	deadline := time.Now().Add(window)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	for time.Now().Before(deadline) {
		info, ok := sup.Status(procID)
		if !ok {
			return fmt.Errorf("process not found")
		}
		switch info.State {
		case supervisor.ProcStopped, supervisor.ProcExited, supervisor.ProcCrashed:
			return fmt.Errorf("state=%s exit=%s", info.State, strings.TrimSpace(info.LastExitErr))
		}
		time.Sleep(120 * time.Millisecond)
	}
	return nil
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

func logPluginGatewayLaunchEnv(ctx context.Context, pluginID string, env map[string]string) {
	if env == nil {
		env = map[string]string{}
	}
	logger.InfoF(ctx, "[plugin-enable] plugin=%s launch_env gateway_base_url_present=%t sts_client_present=%t grpc_upstream_present=%t auth_scheme=%s host_values_present=%t host_values=%s",
		pluginID,
		strings.TrimSpace(env["PX_GATEWAY_BASE_URL"]) != "",
		strings.TrimSpace(env["POWERX_STS_CLIENT_ID"]) != "",
		strings.TrimSpace(env["POWERX_GRPC_UPSTREAM_ADDRESS"]) != "",
		strings.TrimSpace(env["PX_GATEWAY_AUTH_SCHEME"]),
		strings.TrimSpace(env["POWERX_PLUGIN_HOST_VALUES"]) != "",
		strings.TrimSpace(env["POWERX_PLUGIN_HOST_VALUES"]),
	)
}

func applyWSContractEnv(env map[string]string, cfg *config.Config) {
	if env == nil || cfg == nil {
		return
	}
	baseURL := strings.TrimRight(strings.TrimSpace(env["PX_GATEWAY_BASE_URL"]), "/")
	if baseURL == "" {
		return
	}
	env["NUXT_PUBLIC_POWERX_CORE_BASE"] = baseURL

	wsPath := strings.TrimSpace(env["NUXT_PUBLIC_WS_PATH"])
	if wsPath == "" {
		wsPath = "/api/ws"
	}
	env["NUXT_PUBLIC_WS_PATH"] = wsPath

	wsBase := strings.TrimSpace(os.Getenv("POWERX_GATEWAY_WS_BASE_URL"))
	if wsBase == "" {
		host := "127.0.0.1"
		if h := strings.TrimSpace(cfg.Server.Host); h != "" && h != "0.0.0.0" && h != "::" {
			host = h
		}
		port := cfg.Server.Port
		if port <= 0 {
			port = 8080
		}
		wsBase = fmt.Sprintf("ws://%s:%d", host, port)
	}
	wsBase = strings.TrimRight(wsBase, "/")
	env["NUXT_PUBLIC_WS_ORIGIN"] = wsBase
}

func (m *managerImpl) probeGatewayContract(
	ctx context.Context,
	pluginID, apiBaseURL, apiHealthPath string,
	env map[string]string,
	hasTenant bool,
	tenantUUID string,
) error {
	if err := waitHealthy(ctx, apiBaseURL, apiHealthPath, 300*time.Millisecond, 1200*time.Millisecond); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	policy := resolveGatewayProbePolicy(env)
	if policy.TenantScoped && !hasTenant {
		emitGatewayContractAudit(ctx, pluginID, map[string]any{
			"tenant_uuid_present": false,
		}, "dry_run_skipped_no_tenant")
		return nil
	}
	if policy.AuthRequired {
		emitGatewayContractAudit(ctx, pluginID, map[string]any{
			"tenant_uuid_present": hasTenant,
			"auth_scheme":         strings.TrimSpace(env["PX_GATEWAY_AUTH_SCHEME"]),
		}, "dry_run_skipped_sts_only")
		return nil
	}

	baseURL := strings.TrimRight(strings.TrimSpace(env["PX_GATEWAY_BASE_URL"]), "/")
	if baseURL == "" {
		return fmt.Errorf("GW_CFG_MISSING_BASE_URL")
	}
	path := strings.TrimSpace(policy.Path)
	if path == "" {
		return fmt.Errorf("GW_CFG_INVALID_PROBE_PATH")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	target := baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("build dry-run request failed: %w", err)
	}
	if policy.TenantScoped {
		canonicalTenantUUID, err := reqctx.CanonicalTenantUUID(tenantUUID)
		if err != nil {
			return fmt.Errorf("GW_CFG_INVALID_TENANT_UUID: %w", err)
		}
		req.Header.Set("X-PowerX-Tenant", canonicalTenantUUID)
		req.Header.Set("tenant_uuid", canonicalTenantUUID)
	}

	resp, err := (&http.Client{Timeout: 1800 * time.Millisecond}).Do(req)
	if err != nil {
		return fmt.Errorf("dry-run request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 384))
		return fmt.Errorf("dry-run status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

type gatewayProbePolicy struct {
	Path         string
	AuthRequired bool
	TenantScoped bool
}

func resolveGatewayProbePolicy(env map[string]string) gatewayProbePolicy {
	p := gatewayProbePolicy{
		Path:         "/api/v1/tenant/capabilities?page_size=1",
		AuthRequired: true,
		TenantScoped: true,
	}
	if env == nil {
		return p
	}
	if v := strings.TrimSpace(env["PX_GATEWAY_PROBE_PATH"]); v != "" {
		p.Path = v
	}
	if v, ok := parseBoolEnv(env["PX_GATEWAY_PROBE_AUTH_REQUIRED"]); ok {
		p.AuthRequired = v
	}
	if v, ok := parseBoolEnv(env["PX_GATEWAY_PROBE_TENANT_SCOPED"]); ok {
		p.TenantScoped = v
	}
	return p
}

func parseBoolEnv(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true, true
	case "0", "false", "no", "n", "off":
		return false, true
	default:
		return false, false
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
