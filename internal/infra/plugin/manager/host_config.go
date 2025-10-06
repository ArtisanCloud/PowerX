package manager

import (
	"crypto/rand"
	"database/sql"
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
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	corexdb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

const hostValuesFileName = "host-values.yaml"

func (m *managerImpl) generateHostConfig(man plugin_mgr.Manifest, destRoot string, seed *plugin_mgr.HostConfig) (*plugin_mgr.HostConfig, error) {
	cfgDir := filepath.Join(destRoot, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return nil, err
	}

	envAll := m.collectSystemEnv()
	bindOverride := strings.TrimSpace(envAll["POWERX_HTTP_ADDR"])
	envAll["POWERX_PLUGIN_CONFIG_DIR"] = cfgDir
	selected := mergeEnvWithRuntime(envAll, man.Runtime.Env)

	// 若宿主未显式指定 POWERX_HTTP_ADDR，则允许插件根据 PORT 环境变量动态监听
	if bindOverride == "" {
		delete(selected, "POWERX_HTTP_ADDR")
	}
	fmt.Printf("[plugin-host-config] plugin=%s cfg_dir=%s bind_override=%q runtime_bind=%q\n",
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

	// 注入数据库 DSN + Schema（若宿主配置可用）
	dbSection, err := m.buildDatabaseSection(man.ID)
	if err != nil {
		return nil, err
	}
	if dbSection != nil {
		if dbSection.DSN != "" {
			setNestedValue(structured, []string{"database", "dsn"}, dbSection.DSN)
			selected["POWERX_DB_DSN"] = dbSection.DSN
		}
		if dbSection.Schema != "" {
			setNestedValue(structured, []string{"database", "schema"}, dbSection.Schema)
			selected["POWERX_PLUGIN_DB_SCHEMA"] = dbSection.Schema
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
		setNestedValue(structured, []string{"database", "managed"}, true)
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
	if lvl := selected["POWERX_LOG_LEVEL"]; lvl != "" {
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

	// 插件 API 网关安全配置：默认使用宿主 JWT 模式
	if cfg := m.opts.CoreConfig; cfg != nil {
		jwtSecret := strings.TrimSpace(cfg.Auth.JWTSecret)
		issuer := strings.TrimSpace(cfg.Auth.Issuer)
		if jwtSecret != "" && issuer != "" {
			audience := "plugin:" + man.ID
			setNestedValue(structured, []string{"security", "mode"}, "jwt")
			setNestedValue(structured, []string{"security", "jwt", "issuer"}, issuer)
			setNestedValue(structured, []string{"security", "jwt", "audience"}, audience)
			setNestedValue(structured, []string{"security", "jwt", "secret"}, jwtSecret)
			setNestedValue(structured, []string{"security", "jwt", "scope"}, "access")
			setNestedValue(structured, []string{"security", "ctx_hmac", "secret"}, jwtSecret)

			selected["POWERX_SECURITY_MODE"] = "jwt"
			selected["POWERX_SECURITY_JWT_SECRET"] = jwtSecret
			selected["POWERX_SECURITY_JWT_ISSUER"] = issuer
			selected["POWERX_SECURITY_JWT_AUDIENCE"] = audience
			selected["POWERX_SECURITY_JWT_SCOPE"] = "access"
			selected["POWERX_SECURITY_CTX_HMAC_SECRET"] = jwtSecret
		}
	}

	if seed != nil {
		selected = mergeStringMapMissing(selected, seed.Values)
		structured = mergeHostSpecMissing(structured, seed.Spec)
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

type databaseSection struct {
	Driver     string
	DSN        string
	Schema     string
	User       string
	Password   string
	UserHost   string
	SearchPath string
}

func (m *managerImpl) buildDatabaseSection(pluginID string) (*databaseSection, error) {
	if m.opts.CoreConfig == nil {
		return nil, nil
	}

	dbCfg := m.opts.CoreConfig.Database
	driver := normalizeDriver(dbCfg.Driver)
	if driver == "" {
		driver = "postgres"
	}

	schemaName := makePluginSchema(pluginID)
	if schemaName == "" {
		return nil, nil
	}
	userName := makePluginUser(pluginID)
	password, err := generateStrongPassword(32)
	if err != nil {
		return nil, err
	}

	db, cleanup, err := connectAdminDB(dbCfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if err := ensureSchemaExists(db, driver, schemaName); err != nil {
		return nil, err
	}

	section := &databaseSection{
		Driver:   driver,
		Schema:   schemaName,
		User:     userName,
		Password: password,
	}

	switch driver {
	case "postgres":
		if err := ensurePostgresUser(db, dbCfg, section); err != nil {
			return nil, err
		}
		section.DSN = buildPostgresPluginDSN(dbCfg, section)
		section.SearchPath = section.Schema
	case "mysql":
		if err := ensureMySQLUser(db, section); err != nil {
			return nil, err
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
		base = "plugin"
	}
	name := "px_" + base
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.Trim(name, "_")
}

func makePluginUser(id string) string {
	base := pluginSlug(id)
	if base == "" {
		base = "plugin"
	}
	name := "pxu_" + base
	if len(name) > 63 {
		name = name[:63]
	}
	return strings.Trim(name, "_")
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
			return u.String()
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

func (m *managerImpl) cleanupPluginDatabaseResources(hostCfg *plugin_mgr.HostConfig) error {
	if hostCfg == nil || m.opts.CoreConfig == nil {
		return nil
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
