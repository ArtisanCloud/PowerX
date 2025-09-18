package manager

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	corexdb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

const hostValuesFileName = "host-values.yaml"

func (m *managerImpl) generateHostConfig(man plugin_mgr.Manifest, destRoot string) (*plugin_mgr.HostConfig, error) {
	envAll := m.collectSystemEnv()
	selected := mergeEnvWithRuntime(envAll, man.Runtime.Env)

	cfgDir := filepath.Join(destRoot, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return nil, err
	}

	// 确保插件进程可感知宿主提供的配置目录和 host-values 文件
	selected["PX_PLUGIN_CONFIG_DIR"] = cfgDir

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

	// 注入数据库 DSN + Schema（若宿主配置可用）
	if ds, schemaName, err := m.buildDatabaseSection(man.ID); err == nil {
		if ds != "" {
			setNestedValue(structured, []string{"database", "dsn"}, ds)
		}
		if schemaName != "" {
			setNestedValue(structured, []string{"database", "schema"}, schemaName)
			selected["POWERX_PLUGIN_DB_SCHEMA"] = schemaName
		}
	} else {
		return nil, err
	}

	// Server 部分：尽量复用已有配置，未提供则回落到示例默认值
	if bind := selected["PX_BIND_ADDR"]; bind != "" {
		setNestedValue(structured, []string{"server", "bind_addr"}, bind)
	}
	if lvl := selected["PX_LOG_LEVEL"]; lvl != "" {
		setNestedValue(structured, []string{"server", "log_level"}, lvl)
	}
	if devMode, ok := parseBoolish(selected["PX_DEV_MODE"]); ok {
		setNestedValue(structured, []string{"server", "dev_mode"}, devMode)
	}

	// runtime.run_migrate 默认开启，确保首次启用自动迁移
	setNestedValue(structured, []string{"runtime", "run_migrate"}, true)

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
	return mergeEnvWithRuntime(envAll, requested)
}

func mergeEnvWithRuntime(env map[string]string, runtime map[string]string) map[string]string {
	out := make(map[string]string)
	hasRuntime := false
	for k, v := range runtime {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		hasRuntime = true
		out[key] = v
		if hv, ok := env[key]; ok {
			out[key] = hv
		}
	}
	if !hasRuntime {
		return cloneStringMap(env)
	}
	return out
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
		if cfg.Server.SecretKey != "" {
			env["POWERX_SERVER_SECRET_KEY"] = cfg.Server.SecretKey
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
		if dbCfg.Timezone != "" {
			env["POWERX_DB_TIMEZONE"] = dbCfg.Timezone
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

		bus := cfg.EventBus
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

	return env
}

func (m *managerImpl) buildDatabaseSection(pluginID string) (dsn string, schemaName string, err error) {
	if m.opts.CoreConfig == nil {
		return "", "", nil
	}
	dbCfg := m.opts.CoreConfig.Database
	dsn = makeDatabaseDSN(dbCfg)
	schemaName = makePluginSchema(pluginID)
	if schemaName == "" {
		return dsn, "", nil
	}

	if err := ensureSchemaExists(dbCfg, schemaName); err != nil {
		return "", "", err
	}
	return dsn, schemaName, nil
}

func makePluginSchema(id string) string {
	if strings.TrimSpace(id) == "" {
		return ""
	}
	builder := strings.Builder{}
	builder.Grow(len(id) + 3)
	builder.WriteString("px_")
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.' || r == '-' || r == ':' || r == '/':
			builder.WriteByte('_')
		default:
			builder.WriteByte('_')
		}
	}
	slug := strings.Trim(builder.String(), "_")
	if slug == "" {
		slug = "plugin"
	}
	if len(slug) > 63 {
		slug = slug[:63]
	}
	return slug
}

func ensureSchemaExists(cfg corexdb.DatabaseConfig, schema string) error {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	db, err := database.Connect(cfg)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		driver = "postgres"
	}
	var stmt string
	switch driver {
	case "postgres", "pg", "postgresql":
		stmt = fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quoteIdentifier(driver, schema))
	case "mysql":
		stmt = fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quoteIdentifier(driver, schema))
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
