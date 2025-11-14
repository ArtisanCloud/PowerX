package config

import (
	"fmt"
	agentCfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	grpcCfg "github.com/ArtisanCloud/PowerX/internal/server/grpc"
	mcpCfg "github.com/ArtisanCloud/PowerX/internal/server/mcp/config"
	cacheCfg "github.com/ArtisanCloud/PowerX/pkg/cache"
	dbCfg "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	logCfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"strconv"
	"strings"
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

type SecurityConfig struct {
	// 允许作为父页面的来源（CSP frame-ancestors 白名单）
	// 取值示例： "https://admin.powerx.io", "http://localhost:3030", "https://*.powerx.io", "'self'"
	FrameAncestors []string `yaml:"frame_ancestors"`
}

// CoreX 全局配置
type Config struct {
	Server             ServerConfig             `yaml:"server"`              // HTTP/gRPC 监听与行为
	Auth               AuthConfig               `yaml:"auth"`                // JWT / 认证相关
	EventBus           EventBusConfig           `yaml:"event_bus"`           // 事件总线（local/redis）
	EventFabric        EventFabricConfig        `yaml:"event_fabric"`        // 事件骨干调度配置
	IntegrationGateway IntegrationGatewayConfig `yaml:"integration_gateway"` // 集成网关
	AgentLifecycle     AgentLifecycleConfig     `yaml:"agent_lifecycle"`     // Agent 生命周期治理
	KnowledgeSpace     KnowledgeSpaceConfig     `yaml:"knowledge_space"`     // 知识空间治理
	LowCode            LowCodeConfig            `yaml:"dynamic_form"`        // flow 执行相关
	FeatureGate        FeatureGateConfig        `yaml:"feature_gate"`        // 细粒度开关、license
	DevHotload         DevHotloadConfig         `yaml:"dev_hotload"`
	PluginRelease      PluginReleaseConfig      `yaml:"plugin_release"`
	PluginBootstrap    PluginBootstrapConfig    `yaml:"plugin_bootstrap"`
	PluginDebug        PluginDebugConfig        `yaml:"plugin_debug"`
	Database           dbCfg.DatabaseConfig     `yaml:"database"` // 数据库配置
	Cache              cacheCfg.CacheConfig     `yaml:"cache"`    // 缓存配置
	LogConfig          logCfg.LogConfig         `yaml:"log"`      // 输出配置
	AI                 agentCfg.AIConfig        `yaml:"ai"`
	Agent              agentCfg.AgentConfig     `yaml:"agent"` // 智能体工具注册/限流等
	MCP                mcpCfg.MCPConfig         `yaml:"mcp"`   // MCP 服务器配置
	Plugin             PluginConfig             `yaml:"plugin"`
	Security           SecurityConfig           `yaml:"security"`
	Storage            StorageConfig            `yaml:"storage"`
}

// HTTP服务器配置
type ServerConfig struct {
	Port                int                `yaml:"port"`                  // HTTP 端口
	ReadTimeoutSeconds  int                `yaml:"read_timeout_seconds"`  // 读取超时
	WriteTimeoutSeconds int                `yaml:"write_timeout_seconds"` // 写入超时
	Mode                string             `yaml:"mode"`                  // gin 模式: debug/release
	APIPrefix           string             `yaml:"api_prefix"`            // API 前缀
	WSPrefix            string             `yaml:"ws_prefix"`             // API 前缀
	GRPC                grpcCfg.GRPCConfig `yaml:"grpc"`
	SecretKey           string             `yaml:"secret_key"`
}

// JWT认证配置
type AuthConfig struct {
	JWTSecret        string   `yaml:"jwt_secret"`        // HMAC secret
	Issuer           string   `yaml:"issuer"`            // e.g. powerx-auth
	AudienceUser     string   `yaml:"audience_user"`     // e.g. user
	AudienceCustomer string   `yaml:"audience_customer"` // e.g. customer
	Platforms        []string `yaml:"platforms"`         // e.g. admin,web, miniapp

	AccessTTLStr  string `yaml:"access_ttl"`  // e.g. "15m"
	RefreshTTLStr string `yaml:"refresh_ttl"` // e.g. "336h" (14d)
}

// 事件总线配置
type EventBusConfig struct {
	Type          string `yaml:"type"`           // local / redis
	RedisAddr     string `yaml:"redis_addr"`     // redis 地址
	RedisPassword string `yaml:"redis_password"` // redis 密码
	DedupeTTLSec  int    `yaml:"dedupe_ttl_sec"` // 幂等缓存过期
}

// EventFabricConfig 管理事件骨干的可靠性与调度参数。
type EventFabricConfig struct {
	AckTimeoutSeconds int                            `yaml:"ack_timeout_seconds"` // ACK 超时，超过后进入重试
	DefaultMaxRetry   int                            `yaml:"default_max_retry"`   // 默认最大重试次数
	RedisAddr         string                         `yaml:"redis_addr"`          // 重试/幂等使用的 Redis 地址
	RedisPassword     string                         `yaml:"redis_password"`      // Redis 密码
	RedisDB           int                            `yaml:"redis_db"`            // Redis DB
	RetryKeyPrefix    string                         `yaml:"retry_key_prefix"`    // Sorted Set key 前缀
	ReplayKeyPrefix   string                         `yaml:"replay_key_prefix"`   // 回放任务 key 前缀
	SchedulerInterval int                            `yaml:"scheduler_interval"`  // 重试 worker 扫描间隔（秒）
	Security          EventFabricSecurityConfig      `yaml:"security"`            // 安全配置
	Authorization     EventFabricAuthorizationConfig `yaml:"authorization"`       // 授权域治理配置
}

// EventFabricSecurityConfig 定义 TLS/签名要求。
type EventFabricSecurityConfig struct {
	RequireTLS              bool                             `yaml:"require_tls"`
	SignatureSecret         string                           `yaml:"signature_secret"`
	SignatureHeader         string                           `yaml:"signature_header"`
	TimestampHeader         string                           `yaml:"timestamp_header"`
	SignatureKeyID          string                           `yaml:"signature_key_id"`
	AllowedClockSkewSeconds int                              `yaml:"allowed_clock_skew_seconds"`
	Sandbox                 EventFabricSecuritySandboxConfig `yaml:"sandbox"`
}

// EventFabricSecuritySandboxConfig 描述安全沙箱策略。
type EventFabricSecuritySandboxConfig struct {
	Enforce              bool     `yaml:"enforce"`
	AllowedOutboundHosts []string `yaml:"allowed_outbound_hosts"`
	BlockedHTTPPaths     []string `yaml:"blocked_http_paths"`
	BlockedGRPCMethods   []string `yaml:"blocked_grpc_methods"`
	ForbiddenHeaders     []string `yaml:"forbidden_headers"`
}

// EventFabricAuthorizationConfig 配置授权域缓存、审批与审计行为。
type EventFabricAuthorizationConfig struct {
	CacheTTLSeconds             int                                   `yaml:"cache_ttl_seconds"`              // Redis 层缓存 TTL
	LocalCacheTTLSeconds        int                                   `yaml:"local_cache_ttl_seconds"`        // 进程内缓存 TTL
	RedisAddr                   string                                `yaml:"redis_addr"`                     // 授权缓存 Redis 地址
	RedisPassword               string                                `yaml:"redis_password"`                 // Redis 密码
	RedisDB                     int                                   `yaml:"redis_db"`                       // Redis DB
	CacheInvalidateChannel      string                                `yaml:"cache_invalidate_channel"`       // 缓存失效广播频道
	ChallengeSLASeconds         int                                   `yaml:"challenge_sla_seconds"`          // Challenge 审批 SLA（秒）
	ChallengeTopic              string                                `yaml:"challenge_topic"`                // Kafka 主题
	ChallengeConsumerGroup      string                                `yaml:"challenge_consumer_group"`       // Kafka 消费组
	AlertTopic                  string                                `yaml:"alert_topic"`                    // 安全告警事件主题
	RateLimitPrefix             string                                `yaml:"rate_limit_prefix"`              // 速率限制 Redis 前缀
	TimeoutSweepIntervalSeconds int                                   `yaml:"timeout_sweep_interval_seconds"` // 超时扫描间隔
	AuditRetentionDays          int                                   `yaml:"audit_retention_days"`           // 审计留存天数
	AuditArchiveBucket          string                                `yaml:"audit_archive_bucket"`           // 冷存储桶
	AuditArchivePrefix          string                                `yaml:"audit_archive_prefix"`           // 冷存储前缀
	Secrets                     EventFabricAuthorizationSecretsConfig `yaml:"secrets"`
}

// EventFabricAuthorizationSecretsConfig 定义授权域 KMS 配置。
type EventFabricAuthorizationSecretsConfig struct {
	Provider                string `yaml:"provider"`
	KeyID                   string `yaml:"key_id"`
	RotationIntervalSeconds int    `yaml:"rotation_interval_seconds"`
	CacheTTLSeconds         int    `yaml:"cache_ttl_seconds"`
}

// IntegrationGatewayConfig 配置集成网关的限流与事件主题。
type IntegrationGatewayConfig struct {
	RateLimitPrefix  string                            `yaml:"rate_limit_prefix"`
	DefaultRateLimit IntegrationGatewayRateLimitConfig `yaml:"default_rate_limit"`
	EventTopics      IntegrationGatewayEventTopics     `yaml:"event_topics"`
	RedisAddr        string                            `yaml:"redis_addr"`
	RedisPassword    string                            `yaml:"redis_password"`
	RedisDB          int                               `yaml:"redis_db"`
}

// IntegrationGatewayRateLimitConfig 描述默认限流策略。
type IntegrationGatewayRateLimitConfig struct {
	Limit         uint64 `yaml:"limit"`
	Burst         uint64 `yaml:"burst"`
	WindowSeconds int    `yaml:"window_seconds"`
	Scope         string `yaml:"scope"`
}

// IntegrationGatewayEventTopics 描述事件主题默认值。
type IntegrationGatewayEventTopics struct {
	Created             string `yaml:"created"`
	Updated             string `yaml:"updated"`
	InvocationSucceeded string `yaml:"invocation_succeeded"`
	InvocationFailed    string `yaml:"invocation_failed"`
}

// AgentLifecycleConfig 描述代理生命周期模块运行参数。
type AgentLifecycleConfig struct {
	RedisAddr                string                           `yaml:"redis_addr"`
	RedisPassword            string                           `yaml:"redis_password"`
	RedisDB                  int                              `yaml:"redis_db"`
	CapacityKeyPrefix        string                           `yaml:"capacity_key_prefix"`
	HealthKeyPrefix          string                           `yaml:"health_key_prefix"`
	DefaultCapacityInstances int                              `yaml:"default_capacity_instances"`
	EventTopics              AgentLifecycleEventTopics        `yaml:"event_topics"`
	Notifications            AgentLifecycleNotificationConfig `yaml:"notifications"`
	StateBusTopics           AgentLifecycleStateBusTopics     `yaml:"statebus_topics"`
	ShareReviewDays          int                              `yaml:"share_review_days"`
}

// AgentLifecycleEventTopics 定义生命周期与健康事件主题前缀。
type AgentLifecycleEventTopics struct {
	LifecyclePrefix string `yaml:"lifecycle_prefix"`
	HealthPrefix    string `yaml:"health_prefix"`
}

// AgentLifecycleStateBusTopics 定义 StateBus 主题。
type AgentLifecycleStateBusTopics struct {
	Lifecycle string `yaml:"lifecycle"`
	Health    string `yaml:"health"`
}

// AgentLifecycleNotificationConfig 描述企业 IM 通知的运行参数。
type AgentLifecycleNotificationConfig struct {
	IMWebhook        string `yaml:"im_webhook"`
	RetryIntervalSec int    `yaml:"retry_interval_seconds"`
	RetryMaxAttempts int    `yaml:"retry_max_attempts"`
	HTTPTimeoutSec   int    `yaml:"http_timeout_seconds"`
}

// KnowledgeSpaceConfig 描述知识空间模块运行参数。
type KnowledgeSpaceConfig struct {
	RedisAddr              string                           `yaml:"redis_addr"`
	RedisPassword          string                           `yaml:"redis_password"`
	RedisDB                int                              `yaml:"redis_db"`
	LockKeyPrefix          string                           `yaml:"lock_key_prefix"`
	MetricsKeyPrefix       string                           `yaml:"metrics_key_prefix"`
	DefaultRetentionMonths int                              `yaml:"default_retention_months"`
	ProvisioningSLASeconds int                              `yaml:"provisioning_sla_seconds"`
	IngestionSLASeconds    int                              `yaml:"ingestion_sla_seconds"`
	EventTopics            KnowledgeSpaceEventTopics        `yaml:"event_topics"`
	Notifications          KnowledgeSpaceNotificationConfig `yaml:"notifications"`
	VectorStore            KnowledgeSpaceVectorStoreConfig  `yaml:"vector_store"`
	Delta                  KnowledgeSpaceDeltaConfig        `yaml:"delta"`
	Reports                KnowledgeSpaceReportConfig       `yaml:"reports"`
}

// KnowledgeSpaceEventTopics 定义事件主题。
type KnowledgeSpaceEventTopics struct {
	Provisioning string `yaml:"provisioning"`
	Ingestion    string `yaml:"ingestion"`
	Fusion       string `yaml:"fusion"`
	Feedback     string `yaml:"feedback"`
}

// KnowledgeSpaceNotificationConfig 定义通知渠道。
type KnowledgeSpaceNotificationConfig struct {
	IMWebhook        string `yaml:"im_webhook"`
	RetryIntervalSec int    `yaml:"retry_interval_seconds"`
	RetryMaxAttempts int    `yaml:"retry_max_attempts"`
	HTTPTimeoutSec   int    `yaml:"http_timeout_seconds"`
}

// KnowledgeSpaceVectorStoreConfig 描述多驱动向量库配置。
type KnowledgeSpaceVectorStoreConfig struct {
	Driver   string                                  `yaml:"driver"`
	PgVector KnowledgeSpaceVectorStorePGVectorConfig `yaml:"pgvector"`
	Milvus   KnowledgeSpaceVectorStoreMilvusConfig   `yaml:"milvus"`
	Pinecone KnowledgeSpaceVectorStorePineconeConfig `yaml:"pinecone"`
}

type KnowledgeSpaceDeltaConfig struct {
	SourcesConfig        string  `yaml:"sources_config"`
	PartialReleaseConfig string  `yaml:"partial_release_config"`
	ReportPath           string  `yaml:"report_path"`
	AggregateReportPath  string  `yaml:"aggregate_report_path"`
	SLAMinutes           int     `yaml:"sla_minutes"`
	ApprovalMinutes      int     `yaml:"approval_minutes"`
	DefaultDiffAccuracy  float64 `yaml:"default_diff_accuracy"`
}

type KnowledgeSpaceReportConfig struct {
	FeedbackPath string `yaml:"feedback_path"`
	QABridgePath string `yaml:"qa_bridge_path"`
}

type KnowledgeSpaceVectorStorePGVectorConfig struct {
	DSN              string `yaml:"dsn"`
	Schema           string `yaml:"schema"`
	Table            string `yaml:"table"`
	Dimensions       int    `yaml:"dimensions"`
	EnableMigrations bool   `yaml:"enable_migrations"`
	BatchSize        int    `yaml:"batch_size"`
	Lists            int    `yaml:"ivfflat_lists"`
	TimeoutSeconds   int    `yaml:"timeout_seconds"`
}

type KnowledgeSpaceVectorStoreMilvusConfig struct {
	Endpoint string `yaml:"endpoint"`
	APIKey   string `yaml:"api_key"`
	Project  string `yaml:"project"`
}

type KnowledgeSpaceVectorStorePineconeConfig struct {
	Endpoint  string `yaml:"endpoint"`
	APIKey    string `yaml:"api_key"`
	Index     string `yaml:"index"`
	Namespace string `yaml:"namespace"`
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

	// Auth配置（新）
	if v := os.Getenv("CORE_X_AUTH_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("CORE_X_AUTH_ISSUER"); v != "" {
		cfg.Auth.Issuer = v
	}
	if v := os.Getenv("CORE_X_AUTH_AUDIENCE_USER"); v != "" {
		cfg.Auth.AudienceUser = v
	}
	if v := os.Getenv("CORE_X_AUTH_AUDIENCE_CUSTOMER"); v != "" {
		cfg.Auth.AudienceCustomer = v
	}
	if v := os.Getenv("CORE_X_AUTH_PLATFORMS"); v != "" {
		cfg.Auth.Platforms = []string{"admin"}
	}
	if v := os.Getenv("CORE_X_AUTH_ACCESS_TTL"); v != "" {
		cfg.Auth.AccessTTLStr = v // 解析在 bootstrap 里做
	}
	if v := os.Getenv("CORE_X_AUTH_REFRESH_TTL"); v != "" {
		cfg.Auth.RefreshTTLStr = v
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

	// EventFabric 配置
	if v := os.Getenv("CORE_X_EVENT_FABRIC_ACK_TIMEOUT_SEC"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.EventFabric.AckTimeoutSeconds = t
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_DEFAULT_MAX_RETRY"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.EventFabric.DefaultMaxRetry = t
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_REDIS_ADDR"); v != "" {
		cfg.EventFabric.RedisAddr = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_REDIS_PASSWORD"); v != "" {
		cfg.EventFabric.RedisPassword = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_REDIS_DB"); v != "" {
		if dbIdx, err := strconv.Atoi(v); err == nil {
			cfg.EventFabric.RedisDB = dbIdx
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_RETRY_KEY_PREFIX"); v != "" {
		cfg.EventFabric.RetryKeyPrefix = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_REPLAY_KEY_PREFIX"); v != "" {
		cfg.EventFabric.ReplayKeyPrefix = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_SCHEDULER_INTERVAL"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.EventFabric.SchedulerInterval = t
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_REQUIRE_TLS"); v != "" {
		cfg.EventFabric.Security.RequireTLS = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_SIGNATURE_SECRET"); v != "" {
		cfg.EventFabric.Security.SignatureSecret = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_SIGNATURE_HEADER"); v != "" {
		cfg.EventFabric.Security.SignatureHeader = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_TIMESTAMP_HEADER"); v != "" {
		cfg.EventFabric.Security.TimestampHeader = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_SIGNATURE_KEY_ID"); v != "" {
		cfg.EventFabric.Security.SignatureKeyID = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_ALLOWED_SKEW_SEC"); v != "" {
		if skew, err := strconv.Atoi(v); err == nil {
			cfg.EventFabric.Security.AllowedClockSkewSeconds = skew
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_CACHE_TTL"); v != "" {
		if ttl, err := strconv.Atoi(v); err == nil && ttl > 0 {
			cfg.EventFabric.Authorization.CacheTTLSeconds = ttl
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_LOCAL_CACHE_TTL"); v != "" {
		if ttl, err := strconv.Atoi(v); err == nil && ttl > 0 {
			cfg.EventFabric.Authorization.LocalCacheTTLSeconds = ttl
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_REDIS_ADDR"); v != "" {
		cfg.EventFabric.Authorization.RedisAddr = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_REDIS_PASSWORD"); v != "" {
		cfg.EventFabric.Authorization.RedisPassword = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_REDIS_DB"); v != "" {
		if dbIdx, err := strconv.Atoi(v); err == nil && dbIdx >= 0 {
			cfg.EventFabric.Authorization.RedisDB = dbIdx
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_CACHE_INVALIDATE_CHANNEL"); v != "" {
		cfg.EventFabric.Authorization.CacheInvalidateChannel = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_CHALLENGE_SLA"); v != "" {
		if sla, err := strconv.Atoi(v); err == nil && sla > 0 {
			cfg.EventFabric.Authorization.ChallengeSLASeconds = sla
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_CHALLENGE_TOPIC"); v != "" {
		cfg.EventFabric.Authorization.ChallengeTopic = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_CHALLENGE_CONSUMER_GROUP"); v != "" {
		cfg.EventFabric.Authorization.ChallengeConsumerGroup = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_AUDIT_RETENTION_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days > 0 {
			cfg.EventFabric.Authorization.AuditRetentionDays = days
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_AUDIT_ARCHIVE_BUCKET"); v != "" {
		cfg.EventFabric.Authorization.AuditArchiveBucket = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_AUDIT_ARCHIVE_PREFIX"); v != "" {
		cfg.EventFabric.Authorization.AuditArchivePrefix = v
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

	// Plugin Release 配置
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ENABLE_LOCAL_INSTALL"); v != "" {
		cfg.PluginRelease.FeatureFlags.EnableLocalInstall = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ENABLE_PIPELINE_DEPLOYMENT"); v != "" {
		cfg.PluginRelease.FeatureFlags.EnablePipelineDeployment = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ENABLE_OFFLINE_DISTRIBUTION"); v != "" {
		cfg.PluginRelease.FeatureFlags.EnableOfflineDistribution = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_SESSION_TTL_MINUTES"); v != "" {
		if ttl, err := strconv.Atoi(v); err == nil && ttl > 0 {
			cfg.PluginRelease.LocalInstall.SessionTTLMinutes = ttl
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_MAX_ARTIFACT_SIZE_MB"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			cfg.PluginRelease.LocalInstall.MaxArtifactSizeMB = size
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_APPROVAL_SLA_HOURS"); v != "" {
		if sla, err := strconv.Atoi(v); err == nil && sla > 0 {
			cfg.PluginRelease.Pipeline.ApprovalSLAHours = sla
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_MAX_PARALLEL_RELEASES"); v != "" {
		if maxR, err := strconv.Atoi(v); err == nil && maxR > 0 {
			cfg.PluginRelease.Pipeline.MaxParallelReleases = maxR
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_DEFAULT_ROLLBACK_NOTICE_MINUTES"); v != "" {
		if minutes, err := strconv.Atoi(v); err == nil && minutes > 0 {
			cfg.PluginRelease.Pipeline.DefaultRollbackNotice = minutes
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_CANARY_ROLLBACK_TIMEOUT_SECONDS"); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
			cfg.PluginRelease.Canary.RollbackTimeoutSeconds = seconds
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_CANARY_DEFAULT_BATCH_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			cfg.PluginRelease.Canary.DefaultBatchSize = size
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_CANARY_MAX_BATCHES"); v != "" {
		if count, err := strconv.Atoi(v); err == nil && count > 0 {
			cfg.PluginRelease.Canary.MaxBatches = count
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_OFFLINE_BUCKET"); v != "" {
		cfg.PluginRelease.Distribution.OfflineBucket = v
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_OFFLINE_PREFIX"); v != "" {
		cfg.PluginRelease.Distribution.OfflinePrefix = v
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ESCALATION_THRESHOLD"); v != "" {
		if threshold, err := strconv.Atoi(v); err == nil && threshold > 0 {
			cfg.PluginRelease.Distribution.EscalationThreshold = threshold
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ARTIFACT_RETENTION_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days > 0 {
			cfg.PluginRelease.Distribution.ArtifactRetentionDays = days
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_DASHBOARD_UID"); v != "" {
		cfg.PluginRelease.Observability.DashboardUID = v
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ALERT_RULE_PREFIX"); v != "" {
		cfg.PluginRelease.Observability.AlertRulePrefix = v
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_KPI_ROLLBACK_SECONDS"); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
			cfg.PluginRelease.Observability.KPITargets.CanRollbackWithinSeconds = seconds
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_KPI_HOTLOAD_P95_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			cfg.PluginRelease.Observability.KPITargets.HotloadLatencyP95Ms = ms
		}
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
		cfg.Database.UserName = username
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

	// Storage配置
	if driver := os.Getenv("CORE_X_STORAGE_DEFAULT_DRIVER"); driver != "" {
		cfg.Storage.DefaultDriver = strings.ToLower(driver)
	}
	if ttl := os.Getenv("CORE_X_STORAGE_TTL_SECONDS"); ttl != "" {
		if v, err := strconv.Atoi(ttl); err == nil && v > 0 {
			cfg.Storage.TTLSeconds = int32(v)
		}
	}
	if basePath := os.Getenv("CORE_X_STORAGE_LOCAL_BASE_PATH"); basePath != "" {
		cfg.Storage.Local.BasePath = basePath
	}
	if publicURL := os.Getenv("CORE_X_STORAGE_LOCAL_PUBLIC_BASE_URL"); publicURL != "" {
		cfg.Storage.Local.PublicBaseURL = publicURL
	}
	if endpoint := os.Getenv("CORE_X_STORAGE_S3_ENDPOINT"); endpoint != "" {
		cfg.Storage.S3.Endpoint = endpoint
	}
	if region := os.Getenv("CORE_X_STORAGE_S3_REGION"); region != "" {
		cfg.Storage.S3.Region = region
	}
	if ak := os.Getenv("CORE_X_STORAGE_S3_ACCESS_KEY"); ak != "" {
		cfg.Storage.S3.AccessKey = ak
	}
	if sk := os.Getenv("CORE_X_STORAGE_S3_SECRET_KEY"); sk != "" {
		cfg.Storage.S3.SecretKey = sk
	}
	if st := os.Getenv("CORE_X_STORAGE_S3_SESSION_TOKEN"); st != "" {
		cfg.Storage.S3.SessionToken = st
	}
	if bucket := os.Getenv("CORE_X_STORAGE_S3_BUCKET"); bucket != "" {
		cfg.Storage.S3.Bucket = bucket
	}
	if useSSL := os.Getenv("CORE_X_STORAGE_S3_USE_SSL"); useSSL != "" {
		if v, err := strconv.ParseBool(useSSL); err == nil {
			cfg.Storage.S3.UseSSL = v
		}
	}
	if fps := os.Getenv("CORE_X_STORAGE_S3_FORCE_PATH_STYLE"); fps != "" {
		if v, err := strconv.ParseBool(fps); err == nil {
			cfg.Storage.S3.ForcePathStyle = v
		}
	}
	if domain := os.Getenv("CORE_X_STORAGE_S3_EXTERNAL_DOMAIN"); domain != "" {
		cfg.Storage.S3.ExternalDomain = domain
	}
	if presign := os.Getenv("CORE_X_STORAGE_S3_PRESIGN_ENDPOINT"); presign != "" {
		cfg.Storage.S3.PresignEndpoint = presign
	}

	// 兼容旧的环境变量
	if secret := os.Getenv("CORE_X_JWT_SECRET"); secret != "" && cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = secret
	}
	if port := os.Getenv("CORE_X_PORT"); port != "" && cfg.Server.Port == 8077 {
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
