package manager

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	coreconfig "github.com/ArtisanCloud/PowerX/config"
	corexdb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const hostValuesFileName = "host-values.yaml"

var pluginDatabaseBindingNamespace = uuid.MustParse("28943ca2-58c5-5f53-8df5-a20a162f4452")

func (m *managerImpl) generateHostConfig(man plugin_mgr.Manifest, destRoot string, seed *plugin_mgr.HostConfig) (*plugin_mgr.HostConfig, error) {
	deploymentEnv, err := m.requireDeploymentEnv()
	if err != nil {
		return nil, err
	}
	cfgDir := filepath.Join(destRoot, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return nil, err
	}

	envAll := m.collectSystemEnv()
	bindOverride := strings.TrimSpace(envAll["POWERX_HTTP_ADDR"])
	envAll["POWERX_PLUGIN_CONFIG_DIR"] = cfgDir
	envAll["POWERX_DEPLOYMENT_ENV"] = deploymentEnv
	selected := mergeEnvWithRuntime(envAll, man.Runtime.Env)
	normalizePluginLogEnv(selected)

	// 若宿主未显式指定 POWERX_HTTP_ADDR，则允许插件根据 PORT 环境变量动态监听
	if bindOverride == "" {
		delete(selected, "POWERX_HTTP_ADDR")
	}
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "plugin.host_config"}), "[plugin-host-config] plugin=%s cfg_dir=%s bind_override=%q runtime_bind=%q",
		man.ID, cfgDir, bindOverride, selected["POWERX_HTTP_ADDR"])

	// 确保插件进程可感知宿主提供的配置目录和 host-values 文件
	selected["POWERX_PLUGIN_CONFIG_DIR"] = cfgDir

	valuesPath := filepath.Join(cfgDir, hostValuesFileName)
	examplePath := filepath.Join(cfgDir, "values.example.yaml")
	structured := map[string]any{}
	if data, err := os.ReadFile(examplePath); err == nil {
		_ = yaml.Unmarshal(data, &structured) // 解析失败时保留空结构，稍后兜底
	}
	if structured == nil {
		structured = map[string]any{}
	}

	now := time.Now().UTC()
	structured["generated_at"] = now.Format(time.RFC3339)
	setNestedValue(structured, []string{"deployment", "env"}, deploymentEnv)

	// 注入数据库 DSN + Schema（若宿主配置可用）
	dbSection, err := m.provisionDatabaseSection(man.ID)
	if err != nil {
		return nil, err
	}
	if dbSection != nil {
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{
			"module":         "plugin.database_binding",
			"plugin_id":      dbSection.PluginKey,
			"plugin_uuid":    dbSection.PluginUUID,
			"binding_uuid":   dbSection.BindingUUID,
			"deployment_env": dbSection.DeploymentEnv,
			"schema":         dbSection.Schema,
			"database_user":  dbSection.User,
		}), "[plugin-database-binding] created")
		if dbSection.DSN != "" {
			setNestedValue(structured, []string{"database", "dsn"}, dbSection.DSN)
			selected["POWERX_DB_DSN"] = dbSection.DSN
		}
		if dbSection.Schema != "" {
			setNestedValue(structured, []string{"database", "schema"}, dbSection.Schema)
			selected["POWERX_PLUGIN_DB_SCHEMA"] = dbSection.Schema
			selected["POWERX_DB_SCHEMA"] = dbSection.Schema
		}
		if dbSection.User != "" {
			setNestedValue(structured, []string{"database", "user"}, dbSection.User)
			selected["POWERX_DB_USERNAME"] = dbSection.User
		}
		if dbSection.Password != "" {
			setNestedValue(structured, []string{"database", "password"}, dbSection.Password)
			selected["POWERX_DB_PASSWORD"] = dbSection.Password
		}
		if dbSection.Driver != "" {
			setNestedValue(structured, []string{"database", "driver"}, dbSection.Driver)
		}
		if dbSection.SearchPath != "" {
			setNestedValue(structured, []string{"database", "search_path"}, dbSection.SearchPath)
		}
		if dbSection.UserHost != "" {
			setNestedValue(structured, []string{"database", "user_host"}, dbSection.UserHost)
		}
		setNestedValue(structured, []string{"database", "managed"}, dbSection.Managed)
		setNestedValue(structured, []string{"database", "deployment_env"}, dbSection.DeploymentEnv)
		setNestedValue(structured, []string{"database", "plugin_key"}, dbSection.PluginKey)
		setNestedValue(structured, []string{"database", "plugin_uuid"}, dbSection.PluginUUID)
		setNestedValue(structured, []string{"database", "binding_uuid"}, dbSection.BindingUUID)
		// 共享库回退模式必须显式剔除隔离字段，避免 values.example 里的默认值（例如 public）被误带入清理流程。
		if !dbSection.Managed {
			deleteNestedValue(structured, []string{"database", "schema"})
			deleteNestedValue(structured, []string{"database", "search_path"})
			deleteNestedValue(structured, []string{"database", "user"})
			deleteNestedValue(structured, []string{"database", "password"})
			deleteNestedValue(structured, []string{"database", "user_host"})
			delete(selected, "POWERX_PLUGIN_DB_SCHEMA")
			delete(selected, "POWERX_DB_SCHEMA")
		}
	}

	// Server 部分：仅在宿主显式指定时覆盖 bind_addr，避免固定端口
	if bindOverride != "" {
		setNestedValue(structured, []string{"server", "bind_addr"}, bindOverride)
	} else if serverMap, ok := structured["server"].(map[string]any); ok {
		delete(serverMap, "bind_addr")
		if len(serverMap) == 0 {
			delete(structured, "server")
		}
	}
	if lvl := firstNonEmptyMapValue(selected, "POWERX_PLUGIN_LOG_LEVEL", "POWERX_LOG_LEVEL"); lvl != "" {
		setNestedValue(structured, []string{"server", "log_level"}, lvl)
	}
	if devMode, ok := parseBoolish(selected["POWERX_DEV_MODE"]); ok {
		setNestedValue(structured, []string{"server", "dev_mode"}, devMode)
	} else {
		// 2) 没有显式环境开关 → 回落到宿主 CoreConfig（由你的 config.yaml 读入）
		if m.opts.CoreConfig != nil {
			cfg := m.opts.CoreConfig
			setNestedValue(structured, []string{"server", "dev_mode"}, cfg.Plugin.DevMode)
		}
	}

	// runtime.run_migrate 默认开启，确保首次启用自动迁移
	setNestedValue(structured, []string{"runtime", "run_migrate"}, true)
	m.applyHostCORSContract(selected, structured)

	if seed != nil {
		selected = mergeStringMapOverride(selected, seed.Values)
		structured = mergeHostSpecMissing(structured, seed.Spec)
	}
	m.applyDelegatedHostContract(selected, structured, man.ID, nil)
	m.applyHostCORSContract(selected, structured)
	normalizePluginLogEnv(selected)

	// 插件 API 网关安全配置：默认使用宿主 JWT 模式，需覆盖 seed/旧配置
	if cfg := m.opts.CoreConfig; cfg != nil {
		jwtSecret := strings.TrimSpace(cfg.Auth.JWTSecret)
		issuer := strings.TrimSpace(cfg.Auth.Issuer)
		if jwtSecret != "" && issuer != "" {
			audience := "plugin:" + man.ID
			ttl := strings.TrimSpace(cfg.Auth.AccessTTLStr)
			if ttl == "" {
				ttl = "15m"
			}
			setNestedValue(structured, []string{"security", "mode"}, "jwt")
			setNestedValue(structured, []string{"security", "jwt", "issuer"}, issuer)
			setNestedValue(structured, []string{"security", "jwt", "audience"}, audience)
			setNestedValue(structured, []string{"security", "jwt", "secret"}, jwtSecret)
			setNestedValue(structured, []string{"security", "jwt", "scope"}, "access")
			setNestedValue(structured, []string{"security", "ctx_hmac", "secret"}, jwtSecret)
			setNestedValue(structured, []string{"context", "hmac_secret"}, jwtSecret)
			setNestedValue(structured, []string{"context", "issuer"}, issuer)
			setNestedValue(structured, []string{"context", "audience"}, audience)
			setNestedValue(structured, []string{"context", "ttl"}, ttl)

			selected["POWERX_SECURITY_MODE"] = "jwt"
			selected["POWERX_SECURITY_JWT_SECRET"] = jwtSecret
			selected["POWERX_SECURITY_JWT_ISSUER"] = issuer
			selected["POWERX_SECURITY_JWT_AUDIENCE"] = audience
			selected["POWERX_SECURITY_JWT_SCOPE"] = "access"
			selected["POWERX_SECURITY_CTX_HMAC_SECRET"] = jwtSecret
			selected["PLUGIN_CTX_HMAC_SECRET"] = jwtSecret
			selected["POWERX_CTX_ISSUER"] = issuer
			selected["POWERX_CTX_AUDIENCE"] = audience
			selected["POWERX_CTX_TTL"] = ttl
		}
	}

	// 兼容：保留 env 字段供宿主下次启动恢复所需的环境变量
	structured["env"] = selected

	data, err := yaml.Marshal(structured)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(valuesPath, data, 0o640); err != nil {
		return nil, err
	}

	return &plugin_mgr.HostConfig{
		ValuesFile:  valuesPath,
		Values:      cloneStringMap(selected),
		GeneratedAt: now,
		Spec:        stripHostConfigMeta(structured),
	}, nil
}

func (m *managerImpl) provisionDatabaseSection(pluginID string) (*databaseSection, error) {
	if m != nil && m.databaseSectionBuilder != nil {
		return m.databaseSectionBuilder(pluginID)
	}
	return m.buildDatabaseSection(pluginID)
}

// applyDelegatedHostContract enforces delegated_proxy runtime hints in host config.
// Some plugin runtimes prefer reading host-values.yaml over process env.
func (m *managerImpl) applyDelegatedHostContract(selected map[string]string, structured map[string]any, pluginID string, runtimeCred *PluginRuntimeCredential) {
	if selected == nil {
		return
	}
	// 安装产物在宿主内运行，默认按 delegated_proxy 契约写入 taskbus provider=host。
	for _, key := range deprecatedProviderModeEnvKeys() {
		delete(selected, key)
	}
	selected["POWERX_PROXY"] = "1"
	selected["POWERX_PROVIDER_MODE"] = "delegated"
	selected["NUXT_PUBLIC_POWERX_PROVIDER_MODE"] = "delegated"
	selected["NUXT_PUBLIC_POWERX_PROXY"] = "1"
	selected["TASKBUS_PROVIDER"] = "host"
	selected["taskbus_provider"] = "host"
	selected["POWERX_TASKBUS_PROVIDER"] = "host"
	selected["EVENT_BRIDGE_TASKBUS_PROVIDER"] = "host"
	selected["EVENT_BRIDGE_ENABLED"] = "true"
	selected["EVENT_BRIDGE_MODE"] = "taskbus"
	selected["PX_GATEWAY_AUTH_SCHEME"] = "bearer"

	// 内部回连地址与浏览器公开地址分开处理：
	// PX_GATEWAY_BASE_URL 给插件后端进程访问宿主，默认走本机端口；
	// NUXT_PUBLIC_* 会被浏览器执行，必须允许部署侧传入公网域名。
	baseURL := strings.TrimRight(resolveInternalGatewayBaseURL(selected, m), "/")
	if baseURL != "" {
		selected["PX_GATEWAY_BASE_URL"] = baseURL
	}
	publicBaseURL := strings.TrimRight(resolvePublicGatewayBaseURL(selected, baseURL), "/")
	if publicBaseURL != "" {
		selected["NUXT_PUBLIC_POWERX_CORE_BASE"] = publicBaseURL
	}
	if strings.TrimSpace(selected["NUXT_PUBLIC_WS_PATH"]) == "" {
		selected["NUXT_PUBLIC_WS_PATH"] = "/api/ws"
	}
	if m != nil && m.opts.CoreConfig != nil {
		applyWSContractEnv(selected, m.opts.CoreConfig)
	}
	deleteDeprecatedGatewayRuntimeEnv(selected)
	if runtimeCred != nil {
		applyRuntimeCredentialToEnv(selected, runtimeCred)
	} else if m != nil && m.opts.RuntimeCredential != nil {
		if resolved, err := m.opts.RuntimeCredential(context.Background(), strings.TrimSpace(pluginID)); err == nil && resolved != nil {
			applyRuntimeCredentialToEnv(selected, resolved)
		}
	}

	if structured == nil {
		return
	}
	deleteDeprecatedProviderModeKeys(structured)
	setNestedValue(structured, []string{"context", "provider_mode"}, "delegated")
	setNestedValue(structured, []string{"taskbus_provider"}, "host")
	setNestedValue(structured, []string{"event_bridge", "taskbus_provider"}, "host")
	setNestedValue(structured, []string{"event_bridge", "enabled"}, true)
	setNestedValue(structured, []string{"event_bridge", "mode"}, "taskbus")
}

func (m *managerImpl) applyHostCORSContract(selected map[string]string, structured map[string]any) {
	origins := m.hostCORSOrigins()
	if len(origins) == 0 {
		return
	}
	setNestedValue(structured, []string{"host", "web_admin_origins"}, origins)
	if selected != nil {
		selected["POWERX_HOST_WEB_ADMIN_ORIGINS"] = strings.Join(origins, ",")
	}
}

func (m *managerImpl) hostCORSOrigins() []string {
	values := make([]string, 0, 6)
	if m != nil && m.opts.CoreConfig != nil {
		values = append(values, m.opts.CoreConfig.HTTPSecurity.WebAdminOrigins...)
		values = append(values, m.opts.CoreConfig.HTTPSecurity.FrameAncestors...)
		ports := coreconfig.ResolveEffectivePorts(m.opts.CoreConfig)
		if ports.WebAdminPort > 0 {
			values = append(values,
				fmt.Sprintf("http://localhost:%d", ports.WebAdminPort),
				fmt.Sprintf("http://127.0.0.1:%d", ports.WebAdminPort),
			)
		}
	}
	if raw := strings.TrimSpace(os.Getenv("POWERX_PLUGIN_CORS_ORIGINS")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			values = append(values, strings.TrimSpace(part))
		}
	}
	if raw := strings.TrimSpace(os.Getenv("POWERX_WEB_ADMIN_ORIGINS")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			values = append(values, strings.TrimSpace(part))
		}
	}
	return sanitizeHostCORSOrigins(values)
}

func sanitizeHostCORSOrigins(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		origin := strings.TrimSpace(value)
		if origin == "" || origin == "'self'" || strings.Contains(origin, "*") {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		scheme := strings.ToLower(parsed.Scheme)
		if scheme != "http" && scheme != "https" {
			continue
		}
		key := scheme + "://" + strings.ToLower(parsed.Host)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

// ensureDelegatedHostContractForEnable repairs stale host-values before process start.
// This keeps old installed versions self-healing without forcing reinstall.
func (m *managerImpl) ensureDelegatedHostContractForEnable(p *plugin_mgr.Plugin, runtimeCred *PluginRuntimeCredential) error {
	if p == nil {
		return nil
	}
	hvPath := strings.TrimSpace(p.Paths.HostValuesFile)

	hc := p.HostConfig
	if hc == nil && hvPath != "" {
		if loaded, err := loadHostConfig(hvPath); err == nil && loaded != nil {
			hc = loaded
			p.HostConfig = loaded
		}
	}
	if hc == nil {
		hc = &plugin_mgr.HostConfig{}
		p.HostConfig = hc
	}

	values := cloneStringMap(hc.Values)
	spec := cloneAnyMap(hc.Spec)
	if spec == nil {
		spec = map[string]any{}
	}
	m.applyDelegatedHostContract(values, spec, p.ID, runtimeCred)
	m.applyHostCORSContract(values, spec)
	hc.Values = values
	hc.Spec = spec

	if hvPath == "" {
		hc.ValuesFile = hvPath
		return nil
	}

	doc := map[string]any{}
	if raw, err := os.ReadFile(hvPath); err == nil && len(raw) > 0 {
		_ = yaml.Unmarshal(raw, &doc)
	}
	if doc == nil {
		doc = map[string]any{}
	}

	envDoc := map[string]any{}
	if m0, ok := doc["env"].(map[string]any); ok {
		envDoc = cloneAnyMap(m0)
	}
	for k, v := range values {
		envDoc[k] = v
	}
	deleteDeprecatedProviderModeEnvKeys(envDoc)
	doc["env"] = envDoc
	deleteDeprecatedProviderModeKeys(doc)
	setNestedValue(doc, []string{"context", "provider_mode"}, "delegated")
	setNestedValue(doc, []string{"taskbus_provider"}, "host")
	setNestedValue(doc, []string{"event_bridge", "taskbus_provider"}, "host")
	setNestedValue(doc, []string{"event_bridge", "enabled"}, true)
	setNestedValue(doc, []string{"event_bridge", "mode"}, "taskbus")
	m.applyHostCORSContract(values, doc)

	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if err := os.WriteFile(hvPath, data, 0o640); err != nil {
		return err
	}

	hc.ValuesFile = hvPath
	hc.Spec = stripHostConfigMeta(cloneAnyMap(doc))
	return nil
}

func loadHostConfig(path string) (*plugin_mgr.HostConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc := map[string]any{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}

	env := map[string]string{}
	if rawEnv, ok := doc["env"]; ok {
		if m, ok := rawEnv.(map[string]any); ok {
			for k, v := range m {
				if key := strings.TrimSpace(k); key != "" {
					if sv, ok := toString(v); ok {
						env[key] = sv
					}
				}
			}
		}
	}
	delete(doc, "env")

	var gen time.Time
	if ts, ok := doc["generated_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			gen = t
		}
	}
	delete(doc, "generated_at")

	return &plugin_mgr.HostConfig{
		ValuesFile:  path,
		Values:      env,
		GeneratedAt: gen,
		Spec:        cloneAnyMap(doc),
	}, nil
}

func (m *managerImpl) hostEnvForPlugin(p plugin_mgr.Plugin) map[string]string {
	envAll := m.collectSystemEnv()
	requested := make(map[string]string)
	for k, v := range p.Runtime.Env {
		if key := strings.TrimSpace(k); key != "" {
			requested[key] = v
		}
	}
	if p.HostConfig != nil {
		for k, v := range p.HostConfig.Values {
			if key := strings.TrimSpace(k); key != "" {
				requested[key] = v
			}
		}
	}
	out := mergeEnvWithRuntime(envAll, requested)
	normalizePluginLogEnv(out)
	return out
}

func mergeEnvWithRuntime(env map[string]string, runtime map[string]string) map[string]string {
	out := cloneStringMap(env)
	if len(runtime) == 0 {
		return out
	}
	for k, raw := range runtime {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = resolveRuntimeValue(raw, out)
	}
	return out
}

func resolveInternalGatewayBaseURL(selected map[string]string, m *managerImpl) string {
	for _, key := range []string{
		"POWERX_INTERNAL_GATEWAY_BASE_URL",
		"POWERX_HTTP_PROXY_BASE",
		"POWERX_GATEWAY_BASE_URL",
	} {
		if baseURL := strings.TrimSpace(os.Getenv(key)); baseURL != "" {
			return baseURL
		}
	}
	if selected != nil {
		if baseURL := strings.TrimSpace(selected["PX_GATEWAY_BASE_URL"]); baseURL != "" {
			return baseURL
		}
	}
	if m != nil && m.opts.CoreConfig != nil && m.opts.CoreConfig.Server.Port > 0 {
		return fmt.Sprintf("http://127.0.0.1:%d", m.opts.CoreConfig.Server.Port)
	}
	return ""
}

func resolvePublicGatewayBaseURL(env map[string]string, fallback string) string {
	for _, key := range []string{
		"POWERX_PUBLIC_BASE_URL",
		"POWERX_EXTERNAL_BASE_URL",
		"POWERX_GATEWAY_PUBLIC_BASE_URL",
		"POWERX_GATEWAY_BASE_URL",
	} {
		if baseURL := strings.TrimSpace(os.Getenv(key)); baseURL != "" {
			return baseURL
		}
	}
	if env != nil {
		if baseURL := strings.TrimSpace(env["NUXT_PUBLIC_POWERX_CORE_BASE"]); baseURL != "" {
			return baseURL
		}
	}
	return strings.TrimSpace(fallback)
}

var placeholderPattern = regexp.MustCompile(`^\$\{([^}:]+)(?::-(.*))?}$`)

func resolveRuntimeValue(raw string, env map[string]string) string {
	val := strings.TrimSpace(raw)
	if !strings.Contains(val, "${") {
		return val
	}
	if m := placeholderPattern.FindStringSubmatch(val); m != nil {
		key := m[1]
		fallback := ""
		if len(m) > 2 {
			fallback = m[2]
		}
		if v, ok := env[key]; ok && strings.TrimSpace(v) != "" {
			return v
		}
		if v := os.Getenv(key); strings.TrimSpace(v) != "" {
			return v
		}
		return fallback
	}
	return os.Expand(val, func(name string) string {
		if v, ok := env[name]; ok {
			return v
		}
		return os.Getenv(name)
	})
}

func (m *managerImpl) collectSystemEnv() map[string]string {
	env := make(map[string]string)

	if cfg := m.opts.CoreConfig; cfg != nil {
		if cfg.Server.Port > 0 {
			env["POWERX_SERVER_PORT"] = strconv.Itoa(cfg.Server.Port)
		}
		if cfg.Server.APIPrefix != "" {
			env["POWERX_SERVER_API_PREFIX"] = cfg.Server.APIPrefix
		}
		if cfg.Server.Mode != "" {
			env["POWERX_SERVER_MODE"] = cfg.Server.Mode
		}
		// 插件日志控制：默认继承宿主 server mode 作为 gin mode（可被环境变量覆盖）
		if cfg.Server.Mode != "" {
			env["POWERX_PLUGIN_GIN_MODE"] = cfg.Server.Mode
			env["POWERX_GIN_MODE"] = cfg.Server.Mode
		}
		if cfg.Server.SecretKey != "" {
			env["POWERX_SERVER_SECRET_KEY"] = cfg.Server.SecretKey
		}
		if lvl := strings.TrimSpace(cfg.LogConfig.Level); lvl != "" {
			env["POWERX_PLUGIN_LOG_LEVEL"] = lvl
			env["POWERX_LOG_LEVEL"] = lvl
		}

		dbCfg := cfg.Database
		driver := strings.TrimSpace(dbCfg.Driver)
		if driver == "" {
			driver = "postgres"
		}
		env["POWERX_DB_DRIVER"] = driver
		if dbCfg.Host != "" {
			env["POWERX_DB_HOST"] = dbCfg.Host
		}
		if dbCfg.Port > 0 {
			env["POWERX_DB_PORT"] = strconv.Itoa(dbCfg.Port)
		}
		if dbCfg.UserName != "" {
			env["POWERX_DB_USERNAME"] = dbCfg.UserName
		}
		if dbCfg.Password != "" {
			env["POWERX_DB_PASSWORD"] = dbCfg.Password
		}
		if dbCfg.Database != "" {
			env["POWERX_DB_DATABASE"] = dbCfg.Database
		}
		if dbCfg.SSLMode != "" {
			env["POWERX_DB_SSLMODE"] = dbCfg.SSLMode
		}
		if dbCfg.TablePrefix != "" {
			env["POWERX_DB_TABLE_PREFIX"] = dbCfg.TablePrefix
		}
		if dsn := makeDatabaseDSN(dbCfg); dsn != "" {
			env["POWERX_DB_DSN"] = dsn
		}

		cacheCfg := cfg.Cache
		if cacheCfg.Driver != "" {
			env["POWERX_CACHE_DRIVER"] = cacheCfg.Driver
		}
		if cacheCfg.Host != "" {
			env["POWERX_REDIS_HOST"] = cacheCfg.Host
			addr := cacheCfg.Host
			if cacheCfg.Port > 0 {
				addr = fmt.Sprintf("%s:%d", cacheCfg.Host, cacheCfg.Port)
				env["POWERX_REDIS_PORT"] = strconv.Itoa(cacheCfg.Port)
			}
			env["POWERX_REDIS_ADDR"] = addr
		} else if cacheCfg.Port > 0 {
			env["POWERX_REDIS_PORT"] = strconv.Itoa(cacheCfg.Port)
		}
		env["POWERX_REDIS_DB"] = strconv.Itoa(cacheCfg.DB)
		if cacheCfg.Password != "" {
			env["POWERX_REDIS_PASSWORD"] = cacheCfg.Password
		}

		bus := cfg.Event.Bus
		if bus.Type != "" {
			env["POWERX_EVENT_BUS_TYPE"] = bus.Type
		}
		if bus.RedisAddr != "" {
			env["POWERX_EVENT_BUS_REDIS_ADDR"] = bus.RedisAddr
		}
		if bus.RedisPassword != "" {
			env["POWERX_EVENT_BUS_REDIS_PASSWORD"] = bus.RedisPassword
		}
	}

	if m.opts.BasePrefix != "" {
		env["POWERX_PLUGIN_BASE_PREFIX"] = m.opts.BasePrefix
	}
	if m.opts.InstalledRoot != "" {
		env["POWERX_PLUGIN_INSTALLED_ROOT"] = m.opts.InstalledRoot
	}
	if m.opts.RegistryFile != "" {
		env["POWERX_PLUGIN_REGISTRY_FILE"] = m.opts.RegistryFile
	}

	// 宿主进程显式环境变量优先级最高：用于统一控制插件日志行为。
	// 推荐使用 POWERX_PLUGIN_*，同时兼容 POWERX_*。
	for _, k := range []string{
		"POWERX_PLUGIN_HTTP_LOG",
		"POWERX_PLUGIN_GIN_MODE",
		"POWERX_PLUGIN_LOG_LEVEL",
		"POWERX_HTTP_LOG",
		"POWERX_GIN_MODE",
		"POWERX_LOG_LEVEL",
	} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			env[k] = v
		}
	}
	normalizePluginLogEnv(env)

	return env
}

func normalizePluginLogEnv(env map[string]string) {
	if len(env) == 0 {
		return
	}
	httpLog := firstNonEmptyMapValue(env, "POWERX_PLUGIN_HTTP_LOG", "POWERX_HTTP_LOG")
	ginMode := normalizeGinModeValue(firstNonEmptyMapValue(env, "POWERX_PLUGIN_GIN_MODE", "POWERX_GIN_MODE"))
	logLevel := firstNonEmptyMapValue(env, "POWERX_PLUGIN_LOG_LEVEL", "POWERX_LOG_LEVEL")

	if httpLog != "" {
		env["POWERX_PLUGIN_HTTP_LOG"] = httpLog
		env["POWERX_HTTP_LOG"] = httpLog
	}
	if ginMode != "" {
		env["POWERX_PLUGIN_GIN_MODE"] = ginMode
		env["POWERX_GIN_MODE"] = ginMode
		// 兼容只读取 POWERX_SERVER_MODE 的旧插件运行时。
		env["POWERX_SERVER_MODE"] = ginMode
	}
	if logLevel != "" {
		env["POWERX_PLUGIN_LOG_LEVEL"] = logLevel
		env["POWERX_LOG_LEVEL"] = logLevel
	}
}

func normalizeGinModeValue(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "debug", "release", "test":
		return v
	case "true", "1", "yes", "on":
		return "debug"
	case "false", "0", "no", "off":
		return "release"
	default:
		return ""
	}
}

func firstNonEmptyMapValue(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(m[k]); v != "" {
			return v
		}
	}
	return ""
}

type databaseSection struct {
	Driver        string
	DSN           string
	Schema        string
	User          string
	Password      string
	UserHost      string
	SearchPath    string
	Managed       bool
	DeploymentEnv string
	PluginKey     string
	PluginUUID    string
	BindingUUID   string
}

func (m *managerImpl) buildDatabaseSection(pluginID string) (*databaseSection, error) {
	deploymentEnv, err := m.requireDeploymentEnv()
	if err != nil {
		return nil, err
	}

	dbCfg := m.opts.CoreConfig.Database
	driver := normalizeDriver(dbCfg.Driver)
	if driver == "" {
		driver = "postgres"
	}

	schemaName := makePluginSchema(pluginID)
	if schemaName == "" {
		return nil, fmt.Errorf("plugin database isolation requires a non-empty plugin id")
	}
	userName := makePluginUser(deploymentEnv, pluginID)
	password, err := generateStrongPassword(32)
	if err != nil {
		return nil, err
	}

	db, cleanup, err := connectAdminDB(dbCfg)
	if err != nil {
		return nil, fmt.Errorf("plugin %s db isolation bootstrap failed: connect admin db: %w", pluginID, err)
	}
	defer cleanup()

	if err := ensureSchemaExists(db, driver, schemaName); err != nil {
		return nil, fmt.Errorf("plugin %s db isolation bootstrap failed: ensure schema %s: %w", pluginID, schemaName, err)
	}

	section := &databaseSection{
		Driver:        driver,
		Schema:        schemaName,
		User:          userName,
		Password:      password,
		Managed:       true,
		DeploymentEnv: deploymentEnv,
		PluginKey:     pluginID,
		PluginUUID:    pluginDatabasePluginUUID(pluginID),
		BindingUUID:   pluginDatabaseBindingUUID(deploymentEnv, pluginID),
	}

	switch driver {
	case "postgres":
		if err := ensurePostgresUser(db, dbCfg, section); err != nil {
			return nil, fmt.Errorf("plugin %s db isolation bootstrap failed: ensure postgres user %s: %w", pluginID, section.User, err)
		}
		// Role 名称在 deployment.env 改造后会发生变化。旧安装保留 Schema
		// 与业务表时，仅有 GRANT 不足以让新 Role 执行插件 AutoMigrate：
		// PostgreSQL 的 ALTER TABLE/SEQUENCE/VIEW 要求对象 owner。由 Core
		// 的数据库管理连接在安装前将该插件 Schema 内对象统一交给目标 Role，
		// 失败则显式阻断，不能带着半授权状态继续运行迁移。
		if err := reconcilePostgresPluginOwnership(db, dbCfg, section); err != nil {
			return nil, fmt.Errorf("plugin %s db isolation bootstrap failed: reconcile postgres ownership for %s: %w", pluginID, section.Schema, err)
		}
		section.DSN = buildPostgresPluginDSN(dbCfg, section)
		section.SearchPath = section.Schema
	case "mysql":
		if err := ensureMySQLUser(db, section); err != nil {
			return nil, fmt.Errorf("plugin %s db isolation bootstrap failed: ensure mysql user %s: %w", pluginID, section.User, err)
		}
		section.DSN = buildMySQLPluginDSN(dbCfg, section)
	default:
		return nil, fmt.Errorf("unsupported database driver for plugin isolation: %s", driver)
	}

	return section, nil
}

func connectAdminDB(cfg corexdb.DatabaseConfig) (*gorm.DB, func(), error) {
	db, err := database.Connect(cfg)
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = sqlDB.Close()
	}
	return db, cleanup, nil
}

func normalizeDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", "postgres", "pg", "postgresql":
		return "postgres"
	case "mysql", "mariadb":
		return "mysql"
	default:
		return strings.ToLower(strings.TrimSpace(driver))
	}
}

func makePluginSchema(id string) string {
	base := pluginSlug(id)
	if base == "" {
		return ""
	}
	name := "px_" + base
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.Trim(name, "_")
}

func makePluginUser(deploymentEnv, id string) string {
	return makePluginRoleName(deploymentEnv, id)
}

func makePluginRoleName(deploymentEnv, id string) string {
	base := pluginSlug(id)
	if base == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(strings.TrimSpace(id)))
	hash8 := hex.EncodeToString(hash[:])[:8]
	fixedLength := len("pxu") + len(deploymentEnv) + len(hash8) + 3
	maxBaseLength := 63 - fixedLength
	if maxBaseLength <= 0 {
		return ""
	}
	if len(base) > maxBaseLength {
		base = strings.TrimRight(base[:maxBaseLength], "_")
	}
	if base == "" {
		return ""
	}
	return "pxu_" + deploymentEnv + "_" + base + "_" + hash8
}

// makeLegacyPluginRoleName 仅用于所有权收敛时识别 deployment.env 改造前
// 的历史插件 Role。它绝不能作为插件运行时的回退身份。
func makeLegacyPluginRoleName(id string) string {
	base := pluginSlug(id)
	if base == "" {
		return ""
	}
	name := "pxu_" + base
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.Trim(name, "_")
}

func pluginDatabasePluginUUID(pluginID string) string {
	return uuid.NewSHA1(pluginDatabaseBindingNamespace, []byte("plugin:"+strings.TrimSpace(pluginID))).String()
}

func pluginDatabaseBindingUUID(deploymentEnv, pluginID string) string {
	key := "binding:" + deploymentEnv + ":" + strings.TrimSpace(pluginID)
	return uuid.NewSHA1(pluginDatabaseBindingNamespace, []byte(key)).String()
}

func (m *managerImpl) requireDeploymentEnv() (string, error) {
	if m == nil || m.opts.CoreConfig == nil {
		return "", errors.New("plugin database isolation requires CoreConfig.deployment.env")
	}
	env := m.opts.CoreConfig.Deployment.Env
	if err := coreconfig.ValidateDeploymentEnv(env); err != nil {
		return "", fmt.Errorf("plugin database isolation: %w", err)
	}
	return env, nil
}

func (m *managerImpl) validatePluginDatabaseBinding(pluginID string, hostCfg *plugin_mgr.HostConfig) error {
	deploymentEnv, err := m.requireDeploymentEnv()
	if err != nil {
		return err
	}
	if hostCfg == nil || hostCfg.Spec == nil {
		return fmt.Errorf("plugin %s database binding is missing; reinstall or run explicit repair", pluginID)
	}
	rawDB, ok := hostCfg.Spec["database"]
	if !ok {
		return fmt.Errorf("plugin %s database binding is missing; reinstall or run explicit repair", pluginID)
	}
	dbSpec, ok := rawDB.(map[string]any)
	if !ok {
		return fmt.Errorf("plugin %s database binding has invalid format", pluginID)
	}
	expected := map[string]string{
		"deployment_env": deploymentEnv,
		"plugin_key":     pluginID,
		"plugin_uuid":    pluginDatabasePluginUUID(pluginID),
		"binding_uuid":   pluginDatabaseBindingUUID(deploymentEnv, pluginID),
		"schema":         makePluginSchema(pluginID),
		"user":           makePluginUser(deploymentEnv, pluginID),
	}
	for key, want := range expected {
		got := getStringFromMap(dbSpec, key)
		if got == "" {
			return fmt.Errorf("plugin %s database binding field database.%s is missing; reinstall or run explicit repair", pluginID, key)
		}
		if got != want {
			return fmt.Errorf("plugin %s database binding mismatch: database.%s=%q, expected %q", pluginID, key, got, want)
		}
	}
	return nil
}

func pluginSlug(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.' || r == '-' || r == ':' || r == '/' || r == '_':
			builder.WriteByte('_')
		default:
			builder.WriteByte('_')
		}
	}
	slug := strings.Trim(builder.String(), "_")
	for strings.Contains(slug, "__") {
		slug = strings.ReplaceAll(slug, "__", "_")
	}
	return slug
}

func ensureSchemaExists(db *gorm.DB, driver, schema string) error {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	var stmt string
	switch driver {
	case "postgres":
		stmt = fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quoteIdentifier(driver, schema))
	case "mysql":
		stmt = fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", quoteIdentifier(driver, schema))
	default:
		return fmt.Errorf("unsupported database driver for schema creation: %s", driver)
	}
	if stmt == "" {
		return errors.New("empty schema creation statement")
	}
	if err := db.Exec(stmt).Error; err != nil {
		return fmt.Errorf("create schema failed: %w", err)
	}
	return nil
}

func ensurePostgresUser(db *gorm.DB, cfg corexdb.DatabaseConfig, section *databaseSection) error {
	if section == nil {
		return nil
	}
	var exists int
	err := db.Raw("SELECT 1 FROM pg_roles WHERE rolname = ?", section.User).Row().Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		stmt := fmt.Sprintf("CREATE ROLE %s WITH LOGIN PASSWORD %s",
			quoteIdentifier(section.Driver, section.User),
			quoteLiteral(section.Password),
		)
		if execErr := db.Exec(stmt).Error; execErr != nil {
			return execErr
		}
	} else if err != nil {
		return err
	} else {
		stmt := fmt.Sprintf("ALTER ROLE %s WITH LOGIN PASSWORD %s",
			quoteIdentifier(section.Driver, section.User),
			quoteLiteral(section.Password),
		)
		if execErr := db.Exec(stmt).Error; execErr != nil {
			return execErr
		}
	}

	if cfg.Database != "" {
		grant := fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s",
			quoteIdentifier(section.Driver, cfg.Database),
			quoteIdentifier(section.Driver, section.User),
		)
		if err := db.Exec(grant).Error; err != nil {
			return err
		}
	}

	if section.Schema != "" {
		schemaIdent := quoteIdentifier(section.Driver, section.Schema)
		roleIdent := quoteIdentifier(section.Driver, section.User)
		stmts := []string{
			fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", schemaIdent, roleIdent),
			fmt.Sprintf("GRANT CREATE ON SCHEMA %s TO %s", schemaIdent, roleIdent),
			fmt.Sprintf("ALTER ROLE %s SET search_path = %s", roleIdent, schemaIdent),
			fmt.Sprintf("GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %s TO %s", schemaIdent, roleIdent),
			fmt.Sprintf("GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA %s TO %s", schemaIdent, roleIdent),
		}
		for _, stmt := range stmts {
			if err := db.Exec(stmt).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

type postgresPluginRelation struct {
	Name string
	Kind string
}

type postgresPluginFunction struct {
	Name      string
	Arguments string
}

// reconcilePostgresPluginOwnership 让插件数据库绑定的目标 Role 成为该插件
// Schema 内全部可迁移对象的 owner。权限授予只能保证 DML；插件迁移的 DDL
// 必须由 owner 执行。所有语句均在一个事务内完成，任一步失败即回滚。
func reconcilePostgresPluginOwnership(db *gorm.DB, cfg corexdb.DatabaseConfig, section *databaseSection) error {
	if db == nil || section == nil {
		return errors.New("postgres ownership reconciliation requires database and database section")
	}
	if strings.TrimSpace(section.Schema) == "" || strings.TrimSpace(section.User) == "" {
		return errors.New("postgres ownership reconciliation requires schema and target role")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		schemaIdent := quoteIdentifier("postgres", section.Schema)
		targetRoleIdent := quoteIdentifier("postgres", section.User)
		legacyRoles, err := postgresPluginSchemaOwnerRoles(tx, section.Schema, section.User)
		if err != nil {
			return err
		}
		if err := validatePostgresPluginOwnershipSources(legacyRoles, cfg, section); err != nil {
			return err
		}

		// 无论 Schema 是新建还是旧安装遗留，都使目标插件 Role 成为 owner。
		// 若 Core 的数据库管理账号无权转移，将在此处以明确错误阻断安装。
		if err := tx.Exec(fmt.Sprintf("ALTER SCHEMA %s OWNER TO %s", schemaIdent, targetRoleIdent)).Error; err != nil {
			return fmt.Errorf("alter schema owner to %s: %w", section.User, err)
		}

		var relations []postgresPluginRelation
		if err := tx.Raw(`
SELECT c.relname AS name, c.relkind AS kind
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ?
  AND c.relkind IN ('r', 'p', 'S', 'v', 'm', 'f')
  -- serial/identity 列的 sequence 由 ALTER TABLE OWNER 连带转移，PostgreSQL
  -- 禁止在其所属表 owner 变更前单独 ALTER SEQUENCE OWNER。
  AND NOT (
    c.relkind = 'S'
    AND EXISTS (
      SELECT 1
      FROM pg_depend d
      WHERE d.objid = c.oid
        AND d.deptype IN ('a', 'i')
    )
  )
ORDER BY CASE c.relkind
  WHEN 'r' THEN 1
  WHEN 'p' THEN 2
  WHEN 'f' THEN 3
  WHEN 'v' THEN 4
  WHEN 'm' THEN 5
  WHEN 'S' THEN 6
  ELSE 99
END, c.relname`, section.Schema).Scan(&relations).Error; err != nil {
			return fmt.Errorf("list schema relations: %w", err)
		}
		for _, relation := range relations {
			relationIdent := schemaIdent + "." + quoteIdentifier("postgres", relation.Name)
			var stmt string
			switch relation.Kind {
			case "S":
				stmt = fmt.Sprintf("ALTER SEQUENCE %s OWNER TO %s", relationIdent, targetRoleIdent)
			case "v":
				stmt = fmt.Sprintf("ALTER VIEW %s OWNER TO %s", relationIdent, targetRoleIdent)
			case "m":
				stmt = fmt.Sprintf("ALTER MATERIALIZED VIEW %s OWNER TO %s", relationIdent, targetRoleIdent)
			case "r", "p", "f":
				stmt = fmt.Sprintf("ALTER TABLE %s OWNER TO %s", relationIdent, targetRoleIdent)
			default:
				return fmt.Errorf("unsupported relation kind %q for %s", relation.Kind, relation.Name)
			}
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("transfer relation %s ownership: %w", relation.Name, err)
			}
		}

		var functions []postgresPluginFunction
		if err := tx.Raw(`
SELECT p.proname AS name, pg_get_function_identity_arguments(p.oid) AS arguments
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = ?
ORDER BY p.proname, pg_get_function_identity_arguments(p.oid)`, section.Schema).Scan(&functions).Error; err != nil {
			return fmt.Errorf("list schema functions: %w", err)
		}
		for _, function := range functions {
			functionIdent := schemaIdent + "." + quoteIdentifier("postgres", function.Name) + "(" + function.Arguments + ")"
			if err := tx.Exec(fmt.Sprintf("ALTER FUNCTION %s OWNER TO %s", functionIdent, targetRoleIdent)).Error; err != nil {
				return fmt.Errorf("transfer function %s ownership: %w", function.Name, err)
			}
		}

		return revokeLegacyPostgresPluginRolePrivileges(tx, cfg, section, legacyRoles)
	})
}

func postgresPluginSchemaOwnerRoles(tx *gorm.DB, schema, targetRole string) ([]string, error) {
	var roles []string
	if err := tx.Raw(`
SELECT DISTINCT r.rolname
FROM pg_roles r
JOIN pg_namespace n ON n.nspowner = r.oid
WHERE n.nspname = ? AND r.rolname <> ?
UNION
SELECT DISTINCT r.rolname
FROM pg_roles r
JOIN pg_class c ON c.relowner = r.oid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ? AND r.rolname <> ?
UNION
SELECT DISTINCT r.rolname
FROM pg_roles r
JOIN pg_proc p ON p.proowner = r.oid
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = ? AND r.rolname <> ?`, schema, targetRole, schema, targetRole, schema, targetRole).Scan(&roles).Error; err != nil {
		return nil, fmt.Errorf("list existing schema object owners: %w", err)
	}
	return roles, nil
}

// validatePostgresPluginOwnershipSources 防止安装流程接管非本插件对象。
// 允许的历史 owner 只有 Core 数据库管理账号（首次由 Core 创建 Schema 时）
// 或该插件部署环境改造前的精确旧 Role；其余 owner 一律要求人工排查，
// 不得以“同名 Schema”作为接管其他主体对象的充分依据。
func validatePostgresPluginOwnershipSources(owners []string, cfg corexdb.DatabaseConfig, section *databaseSection) error {
	coreUser := strings.TrimSpace(cfg.UserName)
	legacyRole := makeLegacyPluginRoleName(section.PluginKey)
	for _, owner := range owners {
		owner = strings.TrimSpace(owner)
		switch owner {
		case "", section.User, coreUser, legacyRole:
			continue
		default:
			return fmt.Errorf("refuse ownership transfer for schema %s: unexpected existing owner %q; expected Core database user %q or legacy plugin role %q", section.Schema, owner, coreUser, legacyRole)
		}
	}
	return nil
}

// revokeLegacyPostgresPluginRolePrivileges 移除旧插件 Role 对目标 Schema
// 的显式权限。仅处理旧插件 Role 命名，避免撤销 Core 数据库管理账号或其他
// 非插件主体的运维权限。
func revokeLegacyPostgresPluginRolePrivileges(tx *gorm.DB, cfg corexdb.DatabaseConfig, section *databaseSection, previousOwners []string) error {

	schemaIdent := quoteIdentifier("postgres", section.Schema)
	for _, role := range previousOwners {
		if role == section.User || role == strings.TrimSpace(cfg.UserName) {
			continue
		}
		if !strings.HasPrefix(role, "pxu_") {
			continue
		}
		roleIdent := quoteIdentifier("postgres", role)
		stmts := []string{
			fmt.Sprintf("REVOKE ALL PRIVILEGES ON SCHEMA %s FROM %s", schemaIdent, roleIdent),
			fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA %s FROM %s", schemaIdent, roleIdent),
			fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA %s FROM %s", schemaIdent, roleIdent),
			fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA %s FROM %s", schemaIdent, roleIdent),
		}
		for _, stmt := range stmts {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("revoke legacy role %s privileges: %w", role, err)
			}
		}
	}
	return nil
}

func ensureMySQLUser(db *gorm.DB, section *databaseSection) error {
	if section == nil {
		return nil
	}
	host := "%"
	section.UserHost = host
	userIdent := mysqlUserIdent(section.User, host)
	createStmt := fmt.Sprintf("CREATE USER IF NOT EXISTS %s IDENTIFIED BY %s", userIdent, quoteLiteral(section.Password))
	if err := db.Exec(createStmt).Error; err != nil {
		return err
	}
	alterStmt := fmt.Sprintf("ALTER USER %s IDENTIFIED BY %s", userIdent, quoteLiteral(section.Password))
	if err := db.Exec(alterStmt).Error; err != nil {
		return err
	}
	if section.Schema != "" {
		grantStmt := fmt.Sprintf(
			"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, DROP ON %s.* TO %s",
			quoteIdentifier(section.Driver, section.Schema), userIdent,
		)
		if err := db.Exec(grantStmt).Error; err != nil {
			return err
		}
	}
	if err := db.Exec("FLUSH PRIVILEGES").Error; err != nil {
		return err
	}
	return nil
}

func buildPostgresPluginDSN(cfg corexdb.DatabaseConfig, section *databaseSection) string {
	if section == nil {
		return ""
	}
	if cfg.DSN != "" {
		if u, err := url.Parse(cfg.DSN); err == nil && u.Scheme != "" {
			u.User = url.UserPassword(section.User, section.Password)
			q := u.Query()
			if section.Schema != "" {
				q.Set("search_path", section.Schema)
			}
			if cfg.SSLMode != "" {
				q.Set("sslmode", cfg.SSLMode)
			}
			if cfg.Timezone != "" {
				q.Set("timezone", cfg.Timezone)
			}
			u.RawQuery = q.Encode()
			dsn := u.String()
			if tz := strings.TrimSpace(cfg.Timezone); tz != "" {
				encoded := url.QueryEscape(tz)
				if encoded != tz {
					dsn = strings.Replace(dsn, "timezone="+encoded, "timezone="+tz, 1)
					dsn = strings.Replace(dsn, "TimeZone="+encoded, "TimeZone="+tz, 1)
				}
			}
			return dsn
		}
	}

	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Port
	if port <= 0 {
		port = 5432
	}
	dbName := cfg.Database
	if dbName == "" {
		dbName = "postgres"
	}
	ssl := strings.TrimSpace(cfg.SSLMode)
	if ssl == "" {
		ssl = "disable"
	}
	tz := strings.TrimSpace(cfg.Timezone)

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(section.User, section.Password),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/" + dbName,
	}
	q := url.Values{}
	q.Set("sslmode", ssl)
	if tz != "" {
		q.Set("timezone", tz)
	}
	if section.Schema != "" {
		q.Set("search_path", section.Schema)
	}
	u.RawQuery = q.Encode()
	dsn := u.String()
	if tz != "" {
		encoded := url.QueryEscape(tz)
		if encoded != tz {
			dsn = strings.Replace(dsn, "timezone="+encoded, "timezone="+tz, 1)
			dsn = strings.Replace(dsn, "TimeZone="+encoded, "TimeZone="+tz, 1)
		}
	}
	return dsn
}

func buildMySQLPluginDSN(cfg corexdb.DatabaseConfig, section *databaseSection) string {
	if section == nil {
		return ""
	}
	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Port
	if port <= 0 {
		port = 3306
	}
	loc := cfg.Timezone
	if loc == "" {
		loc = "Local"
	}

	params := map[string]string{
		"parseTime": "true",
		"loc":       loc,
		"charset":   "utf8mb4",
	}

	dsnCfg := mysqlDriver.Config{
		User:                 section.User,
		Passwd:               section.Password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", host, port),
		DBName:               section.Schema,
		AllowNativePasswords: true,
		Params:               params,
	}
	return dsnCfg.FormatDSN()
}

func quoteIdentifier(driver string, ident string) string {
	switch driver {
	case "mysql":
		ident = strings.ReplaceAll(ident, "`", "``")
		return "`" + ident + "`"
	default: // postgres 系列
		ident = strings.ReplaceAll(ident, "\"", "\"\"")
		return "\"" + ident + "\""
	}
}

func quoteLiteral(value string) string {
	escaped := strings.ReplaceAll(value, "'", "''")
	return "'" + escaped + "'"
}

const pluginPasswordAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"

func generateStrongPassword(length int) (string, error) {
	if length <= 0 {
		length = 32
	}
	buf := make([]byte, length)
	max := big.NewInt(int64(len(pluginPasswordAlphabet)))
	for i := range buf {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = pluginPasswordAlphabet[n.Int64()]
	}
	return string(buf), nil
}

func mysqlUserIdent(user, host string) string {
	u := strings.ReplaceAll(user, "'", "''")
	h := strings.ReplaceAll(host, "'", "''")
	return fmt.Sprintf("'%s'@'%s'", u, h)
}

func (m *managerImpl) cleanupPluginDatabaseResources(pluginID string, hostCfg *plugin_mgr.HostConfig) error {
	if hostCfg == nil || m.opts.CoreConfig == nil {
		return nil
	}
	if !m.opts.CoreConfig.Plugin.AllowDestructiveDBCleanup {
		return nil
	}
	if err := m.validatePluginDatabaseBinding(pluginID, hostCfg); err != nil {
		return err
	}
	if hostCfg.Spec == nil {
		return nil
	}
	rawDB, ok := hostCfg.Spec["database"]
	if !ok {
		return nil
	}
	dbSpec, ok := rawDB.(map[string]any)
	if !ok {
		return nil
	}

	if managed, ok := boolFromMap(dbSpec, "managed"); ok && !managed {
		return nil
	}

	schema := getStringFromMap(dbSpec, "schema")
	user := getStringFromMap(dbSpec, "user")
	userHost := getStringFromMap(dbSpec, "user_host")
	if schema == "" && user == "" {
		return nil
	}

	dbCfg := m.opts.CoreConfig.Database
	driver := normalizeDriver(dbCfg.Driver)
	if driver == "" {
		driver = "postgres"
	}

	db, cleanup, err := connectAdminDB(dbCfg)
	if err != nil {
		return err
	}
	defer cleanup()

	switch driver {
	case "postgres":
		if schema != "" {
			if err := m.assertPluginSchemaSafeToDrop(schema); err != nil {
				return err
			}
			stmt := fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quoteIdentifier(driver, schema))
			if err := db.Exec(stmt).Error; err != nil {
				return err
			}
		}
		if user != "" {
			roleIdent := quoteIdentifier(driver, user)
			var exists int
			err := db.Raw("SELECT 1 FROM pg_roles WHERE rolname = ?", user).Row().Scan(&exists)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil {
				dropOwned := fmt.Sprintf("DROP OWNED BY %s CASCADE", roleIdent)
				if execErr := db.Exec(dropOwned).Error; execErr != nil {
					return execErr
				}
			}
			stmt := fmt.Sprintf("DROP ROLE IF EXISTS %s", roleIdent)
			if err := db.Exec(stmt).Error; err != nil {
				return err
			}
		}
	case "mysql":
		if schema != "" {
			if err := m.assertPluginSchemaSafeToDrop(schema); err != nil {
				return err
			}
			stmt := fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdentifier(driver, schema))
			if err := db.Exec(stmt).Error; err != nil {
				return err
			}
		}
		if user != "" {
			if userHost == "" {
				userHost = "%"
			}
			stmt := fmt.Sprintf("DROP USER IF EXISTS %s", mysqlUserIdent(user, userHost))
			if err := db.Exec(stmt).Error; err != nil {
				return err
			}
			if err := db.Exec("FLUSH PRIVILEGES").Error; err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported database driver for cleanup: %s", driver)
	}
	return nil
}

func (m *managerImpl) assertPluginSchemaSafeToDrop(schema string) error {
	norm := strings.ToLower(strings.TrimSpace(schema))
	if norm == "" {
		return fmt.Errorf("refuse to drop schema/database: empty schema")
	}
	// 保护 PostgreSQL 系统/默认 schema，避免误删宿主核心数据。
	if norm == "public" || norm == "information_schema" || norm == "pg_catalog" || strings.HasPrefix(norm, "pg_") {
		return fmt.Errorf("refuse to drop protected schema/database: %s", schema)
	}
	if m != nil && m.opts.CoreConfig != nil {
		coreDB := strings.ToLower(strings.TrimSpace(m.opts.CoreConfig.Database.Database))
		// 在 MySQL 场景下 schema 字段承载 database 名称，也保护主库名。
		if coreDB != "" && norm == coreDB {
			return fmt.Errorf("refuse to drop core schema/database: %s", schema)
		}
	}
	return nil
}

func getStringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if val, ok := m[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
		if s, ok := toString(val); ok {
			return s
		}
	}
	return ""
}

func boolFromMap(m map[string]any, key string) (bool, bool) {
	if m == nil {
		return false, false
	}
	val, ok := m[key]
	if !ok {
		return false, false
	}
	switch typed := val.(type) {
	case bool:
		return typed, true
	case string:
		return parseBoolish(typed)
	default:
		if s, ok := toString(typed); ok {
			return parseBoolish(s)
		}
	}
	return false, false
}

func setNestedValue(root map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	current := root
	for i, key := range path {
		if i == len(path)-1 {
			current[key] = value
			return
		}
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
}

func deleteNestedValue(root map[string]any, path []string) {
	if len(path) == 0 || root == nil {
		return
	}
	current := root
	for i, key := range path {
		if i == len(path)-1 {
			delete(current, key)
			return
		}
		next, ok := current[key].(map[string]any)
		if !ok {
			return
		}
		current = next
	}
}

func parseBoolish(v string) (bool, bool) {
	s := strings.TrimSpace(strings.ToLower(v))
	switch s {
	case "true", "1", "yes", "on":
		return true, true
	case "false", "0", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func stripHostConfigMeta(doc map[string]any) map[string]any {
	if doc == nil {
		return nil
	}
	clean := cloneAnyMap(doc)
	if clean == nil {
		clean = map[string]any{}
	}
	delete(clean, "env")
	delete(clean, "generated_at")
	return clean
}

func mergeStringMapMissing(dst map[string]string, src map[string]string) map[string]string {
	if dst == nil {
		dst = map[string]string{}
	}
	if len(src) == 0 {
		return dst
	}
	for k, v := range src {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if cur, ok := dst[key]; !ok || strings.TrimSpace(cur) == "" {
			dst[key] = v
		}
	}
	return dst
}

func mergeStringMapOverride(dst map[string]string, src map[string]string) map[string]string {
	if dst == nil {
		dst = map[string]string{}
	}
	for k, v := range src {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		dst[key] = v
	}
	return dst
}

func deleteDeprecatedProviderModeKeys(doc map[string]any) {
	deleteNestedValue(doc, []string{"context", "iam" + "_" + "mode"})
}

func deleteDeprecatedProviderModeEnvKeys(env map[string]any) {
	for _, key := range deprecatedProviderModeEnvKeys() {
		delete(env, key)
	}
}

func deprecatedProviderModeEnvKeys() []string {
	return []string{"IAM" + "_MODE", "IAM" + "Mode"}
}

func mergeHostSpecMissing(dst map[string]any, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	if len(src) == 0 {
		return dst
	}
	for k, v := range src {
		if existing, ok := dst[k]; ok {
			left, lok := existing.(map[string]any)
			right, rok := v.(map[string]any)
			if lok && rok {
				dst[k] = mergeHostSpecMissing(left, right)
			}
			continue
		}
		switch typed := v.(type) {
		case map[string]any:
			dst[k] = cloneAnyMap(typed)
		case []any:
			cloned := make([]any, len(typed))
			for i := range typed {
				cloned[i] = cloneAnyValue(typed[i])
			}
			dst[k] = cloned
		default:
			dst[k] = typed
		}
	}
	return dst
}

func toString(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case fmt.Stringer:
		return val.String(), true
	case int:
		return strconv.Itoa(val), true
	case int64:
		return strconv.FormatInt(val, 10), true
	case uint64:
		return strconv.FormatUint(val, 10), true
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), true
	case bool:
		if val {
			return "true", true
		}
		return "false", true
	default:
		return "", false
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func makeDatabaseDSN(cfg corexdb.DatabaseConfig) string {
	if strings.TrimSpace(cfg.DSN) != "" {
		return cfg.DSN
	}

	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	switch driver {
	case "", "postgres", "pg", "postgresql":
		ssl := strings.TrimSpace(cfg.SSLMode)
		if ssl == "" {
			ssl = "disable"
		}
		tz := strings.TrimSpace(cfg.Timezone)
		if tz == "" {
			tz = "UTC"
		}
		return fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
			cfg.Host, cfg.Port, cfg.UserName, cfg.Password, cfg.Database, ssl, tz,
		)
	case "mysql":
		tz := strings.TrimSpace(cfg.Timezone)
		if tz == "" {
			tz = "Local"
		}
		host := cfg.Host
		if strings.TrimSpace(host) == "" {
			host = "127.0.0.1"
		}
		port := cfg.Port
		if port <= 0 {
			port = 3306
		}
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=%s&charset=utf8mb4",
			cfg.UserName, cfg.Password, host, port, cfg.Database, url.QueryEscape(tz),
		)
	default:
		return cfg.DSN
	}
}
