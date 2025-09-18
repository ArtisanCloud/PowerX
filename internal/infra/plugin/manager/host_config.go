package manager

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	corexdb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

const hostValuesFileName = "host-values.yaml"

type hostValuesFile struct {
	GeneratedAt string            `yaml:"generated_at"`
	Env         map[string]string `yaml:"env"`
}

func (m *managerImpl) generateHostConfig(man plugin_mgr.Manifest, destRoot string) (*plugin_mgr.HostConfig, error) {
	envAll := m.collectSystemEnv()
	selected := mergeEnvWithRuntime(envAll, man.Runtime.Env)
	if len(selected) == 0 {
		return nil, nil
	}

	cfgDir := filepath.Join(destRoot, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	hv := hostValuesFile{
		GeneratedAt: now.Format(time.RFC3339),
		Env:         selected,
	}
	data, err := yaml.Marshal(hv)
	if err != nil {
		return nil, err
	}
	valuesPath := filepath.Join(cfgDir, hostValuesFileName)
	if err := os.WriteFile(valuesPath, data, 0o640); err != nil {
		return nil, err
	}

	return &plugin_mgr.HostConfig{
		ValuesFile:  valuesPath,
		Values:      cloneStringMap(selected),
		GeneratedAt: now,
	}, nil
}

func loadHostConfig(path string) (*plugin_mgr.HostConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var hv hostValuesFile
	if err := yaml.Unmarshal(raw, &hv); err != nil {
		return nil, err
	}
	values := make(map[string]string, len(hv.Env))
	for k, v := range hv.Env {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		values[key] = v
	}
	var gen time.Time
	if hv.GeneratedAt != "" {
		if t, err := time.Parse(time.RFC3339, hv.GeneratedAt); err == nil {
			gen = t
		}
	}
	return &plugin_mgr.HostConfig{ValuesFile: path, Values: values, GeneratedAt: gen}, nil
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
