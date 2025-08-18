package config

import (
	"fmt"
	dbCfg "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	logCfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	agentCfg "github.com/ArtisanCloud/PowerX/services/agent/config"
	mcpCfg "github.com/ArtisanCloud/PowerX/services/mcp/config"
	"log"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// 定义一个全局配置变量
var GlobalConfig *Config

// 初始化全局配置
func InitGlobalConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	GlobalConfig = &config
	return nil
}

// 获取全局配置
func GetGlobalConfig() *Config {
	if GlobalConfig == nil {
		// 初始化全局配置
		if err := InitGlobalConfig("etc/config.yaml"); err != nil {
			log.Fatalf("初始化全局配置失败: %v", err)
		}
	}
	return GlobalConfig
}

// CoreX 全局配置
type Config struct {
	Server      ServerConfig         `yaml:"server"`       // HTTP/gRPC 监听与行为
	Auth        AuthConfig           `yaml:"auth"`         // JWT / 认证相关
	EventBus    EventBusConfig       `yaml:"event_bus"`    // 事件总线（local/redis）
	LowCode     LowCodeConfig        `yaml:"dynamic_form"` // flow 执行相关
	FeatureGate FeatureGateConfig    `yaml:"feature_gate"` // 细粒度开关、license
	Database    dbCfg.DatabaseConfig `yaml:"database"`     // 数据库配置
	LogConfig   logCfg.LogConfig     `yaml:"log"`          // 输出配置
	Agent       agentCfg.AgentConfig `yaml:"agent"`        // 智能体工具注册/限流等
	MCP         mcpCfg.MCPConfig     `yaml:"mcp"`          // MCP 服务器配置
	Plugin      PluginConfig         `yaml:"plugin"`
}

// HTTP服务器配置
type ServerConfig struct {
	Port                int    `yaml:"port"`                  // HTTP 端口
	ReadTimeoutSeconds  int    `yaml:"read_timeout_seconds"`  // 读取超时
	WriteTimeoutSeconds int    `yaml:"write_timeout_seconds"` // 写入超时
	Mode                string `yaml:"mode"`                  // gin 模式: debug/release
	APIPrefix           string `yaml:"api_prefix"`            // API 前缀
}

// JWT认证配置
type AuthConfig struct {
	JWTSecret        string   `yaml:"jwt_secret"`        // HMAC secret 或私钥路径
	ExpectedAudience string   `yaml:"expected_audience"` // 期望 audience，例如 admin/openapi/miniapp
	RequiredScopes   []string `yaml:"required_scopes"`   // 必需 scope
	TokenTTLHours    int      `yaml:"token_ttl_hours"`   // 默认 token 过期小时
}

// 事件总线配置
type EventBusConfig struct {
	Type          string `yaml:"type"`           // local / redis
	RedisAddr     string `yaml:"redis_addr"`     // redis 地址
	RedisPassword string `yaml:"redis_password"` // redis 密码
	DedupeTTLSec  int    `yaml:"dedupe_ttl_sec"` // 幂等缓存过期
}

// 低代码引擎配置
type LowCodeConfig struct {
	MaxConcurrentFlows int `yaml:"max_concurrent_flows"` // 并发 flow 限制
	DefaultTimeoutSec  int `yaml:"default_timeout_sec"`  // 每个 flow 默认超时
}

// 功能开关配置
type FeatureGateConfig struct {
	LicenseKey string `yaml:"license_key"` // license 或灰度控制 token
}

// Load 加载配置文件并合并环境变量
func Load(configPath string) (*Config, error) {
	// 1. 加载默认配置
	cfg := GetDefaults()

	// 2. 从YAML文件加载（如果存在）
	if configPath != "" {
		if err := loadFromYAML(cfg, configPath); err != nil {
			return nil, fmt.Errorf("加载YAML配置失败: %w", err)
		}
	}

	// 3. 从环境变量覆盖
	loadFromEnv(cfg)

	// 4. 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return cfg, nil
}

// loadFromEnv 从环境变量加载配置
func loadFromEnv(cfg *Config) {
	// Server配置
	if port := os.Getenv("CORE_X_SERVER_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Server.Port = p
		}
	}
	if mode := os.Getenv("CORE_X_SERVER_MODE"); mode != "" {
		cfg.Server.Mode = mode
	}
	if timeout := os.Getenv("CORE_X_SERVER_READ_TIMEOUT"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil {
			cfg.Server.ReadTimeoutSeconds = t
		}
	}
	if timeout := os.Getenv("CORE_X_SERVER_WRITE_TIMEOUT"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil {
			cfg.Server.WriteTimeoutSeconds = t
		}
	}

	// Auth配置
	if secret := os.Getenv("CORE_X_AUTH_JWT_SECRET"); secret != "" {
		cfg.Auth.JWTSecret = secret
	}
	if audience := os.Getenv("CORE_X_AUTH_EXPECTED_AUDIENCE"); audience != "" {
		cfg.Auth.ExpectedAudience = audience
	}
	if scopes := os.Getenv("CORE_X_AUTH_REQUIRED_SCOPES"); scopes != "" {
		cfg.Auth.RequiredScopes = strings.Split(scopes, ",")
	}
	if ttl := os.Getenv("CORE_X_AUTH_TOKEN_TTL_HOURS"); ttl != "" {
		if t, err := strconv.Atoi(ttl); err == nil {
			cfg.Auth.TokenTTLHours = t
		}
	}

	// EventBus配置
	if busType := os.Getenv("CORE_X_EVENT_BUS_TYPE"); busType != "" {
		cfg.EventBus.Type = busType
	}
	if redisAddr := os.Getenv("CORE_X_EVENT_BUS_REDIS_ADDR"); redisAddr != "" {
		cfg.EventBus.RedisAddr = redisAddr
	}
	if redisPassword := os.Getenv("CORE_X_EVENT_BUS_REDIS_PASSWORD"); redisPassword != "" {
		cfg.EventBus.RedisPassword = redisPassword
	}
	if ttl := os.Getenv("CORE_X_EVENT_BUS_DEDUPE_TTL_SEC"); ttl != "" {
		if t, err := strconv.Atoi(ttl); err == nil {
			cfg.EventBus.DedupeTTLSec = t
		}
	}

	// LowCode配置
	if maxFlows := os.Getenv("CORE_X_LOW_CODE_MAX_CONCURRENT_FLOWS"); maxFlows != "" {
		if m, err := strconv.Atoi(maxFlows); err == nil {
			cfg.LowCode.MaxConcurrentFlows = m
		}
	}
	if timeout := os.Getenv("CORE_X_LOW_CODE_DEFAULT_TIMEOUT_SEC"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil {
			cfg.LowCode.DefaultTimeoutSec = t
		}
	}

	// AgentTools配置
	if audit := os.Getenv("CORE_X_AGENT_TOOLS_ENABLE_AUDIT"); audit != "" {
		// cfg.AgentTools.EnableAudit = strings.ToLower(audit) == "true"
	}

	// LogConfig配置 - 使用外部logger配置
	// 这里可以根据需要添加对LogConfig字段的环境变量支持
	// 例如：cfg.LogConfig.Level, cfg.LogConfig.Format 等

	// FeatureGate配置
	if license := os.Getenv("CORE_X_FEATURE_GATE_LICENSE_KEY"); license != "" {
		cfg.FeatureGate.LicenseKey = license
	}

	// 数据库配置
	if host := os.Getenv("CORE_X_DB_HOST"); host != "" {
		cfg.Database.Host = host
	}
	if port := os.Getenv("CORE_X_DB_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Database.Port = p
		}
	}
	if username := os.Getenv("CORE_X_DB_USERNAME"); username != "" {
		cfg.Database.Username = username
	}
	if password := os.Getenv("CORE_X_DB_PASSWORD"); password != "" {
		cfg.Database.Password = password
	}
	if database := os.Getenv("CORE_X_DB_DATABASE"); database != "" {
		cfg.Database.Database = database
	}
	if sslMode := os.Getenv("CORE_X_DB_SSL_MODE"); sslMode != "" {
		cfg.Database.SSLMode = sslMode
	}
	if timezone := os.Getenv("CORE_X_DB_TIMEZONE"); timezone != "" {
		cfg.Database.Timezone = timezone
	}
	if tablePrefix := os.Getenv("CORE_X_DB_TABLE_PREFIX"); tablePrefix != "" {
		cfg.Database.TablePrefix = tablePrefix
	}
	if maxIdleConns := os.Getenv("CORE_X_DB_MAX_IDLE_CONNS"); maxIdleConns != "" {
		if m, err := strconv.Atoi(maxIdleConns); err == nil {
			cfg.Database.MaxIdleConns = m
		}
	}
	if maxOpenConns := os.Getenv("CORE_X_DB_MAX_OPEN_CONNS"); maxOpenConns != "" {
		if m, err := strconv.Atoi(maxOpenConns); err == nil {
			cfg.Database.MaxOpenConns = m
		}
	}
	if connMaxLifetime := os.Getenv("CORE_X_DB_CONN_MAX_LIFETIME"); connMaxLifetime != "" {
		if m, err := strconv.Atoi(connMaxLifetime); err == nil {
			cfg.Database.ConnMaxLifetimeMinutes = m
		}
	}
	if logLevel := os.Getenv("CORE_X_DB_LOG_LEVEL"); logLevel != "" {
		cfg.Database.LogLevel = logLevel
	}

	// 兼容旧的环境变量
	if secret := os.Getenv("CORE_X_JWT_SECRET"); secret != "" && cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = secret
	}
	if port := os.Getenv("CORE_X_PORT"); port != "" && cfg.Server.Port == 8080 {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Server.Port = p
		}
	}
	if mode := os.Getenv("GIN_MODE"); mode != "" && cfg.Server.Mode == "debug" {
		cfg.Server.Mode = mode
	}
	if busType := os.Getenv("EVENT_BUS_TYPE"); busType != "" && cfg.EventBus.Type == "local" {
		cfg.EventBus.Type = busType
	}
}
