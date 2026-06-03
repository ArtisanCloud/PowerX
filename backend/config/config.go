package config

import (
	"bufio"
	"context"
	"fmt"
	agentCfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	grpcCfg "github.com/ArtisanCloud/PowerX/internal/server/grpc"
	mcpCfg "github.com/ArtisanCloud/PowerX/internal/server/mcp/config"
	cacheCfg "github.com/ArtisanCloud/PowerX/pkg/cache"
	dbCfg "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	logCfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// 定义一个全局配置变量
var GlobalConfig *Config
var globalConfigPath string

// 初始化全局配置
func InitGlobalConfig(configPath string) error {
	loadDotEnvCandidates(configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}
	loadFromEnv(&config)

	// AI: default fill + global snapshot (read-only)
	config.AI.SetDefaults()
	agentCfg.SetGlobalAIConfig(&config.AI)

	GlobalConfig = &config
	globalConfigPath = configPath
	return nil
}

// 获取全局配置
func GetGlobalConfig() *Config {
	if GlobalConfig == nil {
		// 初始化全局配置，优先级：
		// 1) POWERX_CONFIG 显式指定
		// 2) 可执行文件同级目录下的 etc/config.yaml（适配 dist 产物从任意 cwd 启动）
		// 3) 当前工作目录的 backend/etc/config.yaml（仓库开发默认）
		// 4) 当前工作目录的 etc/config.yaml（兼容历史）
		// 5) 向上查找祖先目录中的 backend/etc/config.yaml
		// 6) 向上查找祖先目录中的 etc/config.yaml
		candidates := make([]string, 0, 6)
		if p := strings.TrimSpace(os.Getenv("POWERX_CONFIG")); p != "" {
			candidates = append(candidates, p)
		}
		if p := configPathNearExecutable("etc/config.yaml"); p != "" {
			candidates = append(candidates, p)
		}
		candidates = append(candidates, "backend/etc/config.yaml")
		candidates = append(candidates, "etc/config.yaml")
		if p := findConfigPath("backend/etc/config.yaml"); p != "" {
			candidates = append(candidates, p)
		}
		if p := findConfigPath("etc/config.yaml"); p != "" {
			candidates = append(candidates, p)
		}

		var lastErr error
		for _, p := range candidates {
			if strings.TrimSpace(p) == "" {
				continue
			}
			if err := InitGlobalConfig(p); err == nil {
				return GlobalConfig
			} else {
				lastErr = err
			}
		}
		logger.ErrorF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "config.global"}), "初始化全局配置失败: %v", lastErr)
		os.Exit(1)
	}
	return GlobalConfig
}

func GetGlobalConfigPath() string {
	path := strings.TrimSpace(globalConfigPath)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func ResolveAPIPrefix(cfg *Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Server.APIPrefix) != "" {
		return strings.TrimSpace(cfg.Server.APIPrefix)
	}
	return "/api"
}

type EffectivePorts struct {
	BackendPort  int `json:"backend_port"`
	WebAdminPort int `json:"web_admin_port"`
}

func ResolveEffectivePorts(cfg *Config) EffectivePorts {
	ports := effectivePortsByEnv()
	if cfg != nil && cfg.Server.Port > 0 {
		ports.BackendPort = cfg.Server.Port
	}
	if cfg != nil && cfg.WebAdminPort > 0 {
		ports.WebAdminPort = cfg.WebAdminPort
	}
	if port := parsePortEnv("POWERX_BACKEND_PORT"); port > 0 {
		ports.BackendPort = port
	}
	if port := parsePortEnv("POWERX_WEB_ADMIN_PORT"); port > 0 {
		ports.WebAdminPort = port
	}
	return ports
}

func effectivePortsByEnv() EffectivePorts {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("POWERX_ENV")))
	if env == "dev" {
		return EffectivePorts{
			BackendPort:  8077,
			WebAdminPort: 3030,
		}
	}
	return EffectivePorts{
		BackendPort:  8080,
		WebAdminPort: 3000,
	}
}

func parsePortEnv(key string) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 || v > 65535 {
		return 0
	}
	return v
}

func findConfigPath(relPath string) string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := filepath.Clean(wd)
	for {
		candidate := filepath.Join(dir, relPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		next := filepath.Dir(dir)
		if next == dir || next == "." || next == string(filepath.Separator) {
			break
		}
		dir = next
	}
	return ""
}

func configPathNearExecutable(relPath string) string {
	exe, err := os.Executable()
	if err != nil || strings.TrimSpace(exe) == "" {
		return ""
	}
	exeDir := filepath.Dir(exe)
	candidate := filepath.Join(exeDir, relPath)
	if _, statErr := os.Stat(candidate); statErr == nil {
		return candidate
	}
	return ""
}

type HTTPSecurityConfig struct {
	// 允许作为父页面的来源（CSP frame-ancestors 白名单）
	// 取值示例： "https://admin.powerx.io", "http://localhost:3030", "https://*.powerx.io", "'self'"
	FrameAncestors []string `yaml:"frame_ancestors"`
	// 浏览器访问 PowerX Web Admin 的公开 Origin，用于插件宿主模式 CORS/Origin 契约。
	WebAdminOrigins []string `yaml:"web_admin_origins"`
}

// TenantConfig 控制租户头部解析与缓存策略。
type TenantConfig struct {
	RequireUUID bool `yaml:"require_uuid"`
}

type InstallConfig struct {
	Status         string `yaml:"status"`
	LockMode       string `yaml:"lock_mode"`
	AllowWithoutDB bool   `yaml:"allow_without_db"`
}

func (c InstallConfig) EffectiveStatus() string {
	status := strings.ToLower(strings.TrimSpace(c.Status))
	switch status {
	case "installed", "configuring", "uninstalled":
		return status
	default:
		// 兼容历史配置未声明 install.status 的场景，默认视为已安装。
		return "installed"
	}
}

func (c InstallConfig) EffectiveLockMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.LockMode))
	if mode == "" {
		return "strict"
	}
	return mode
}

// CoreX 全局配置
type Config struct {
	Version            string                   `yaml:"version"`             // 系统版本（用于权限 introduced 等）
	Server             ServerConfig             `yaml:"server"`              // HTTP/gRPC 监听与行为
	WebAdminPort       int                      `yaml:"web_admin_port"`      // Web Admin 公开访问端口（setup/install 写入）
	Auth               AuthConfig               `yaml:"auth"`                // JWT / 认证相关
	Event              EventConfig              `yaml:"event"`               // 事件配置（系统总线 + Event Fabric）
	Queue              QueueConfig              `yaml:"queue"`               // 全局队列驱动
	Scheduler          SchedulerConfig          `yaml:"scheduler"`           // 全局调度器
	IntegrationGateway IntegrationGatewayConfig `yaml:"integration_gateway"` // 集成网关
	CapabilityRegistry CapabilityRegistryConfig `yaml:"capability_registry"` // Capability Registry 配置
	AgentLifecycle     AgentLifecycleConfig     `yaml:"agent_lifecycle"`     // Agent 生命周期治理
	KnowledgeSpace     KnowledgeSpaceConfig     `yaml:"knowledge_space"`     // 知识空间治理
	LowCode            LowCodeConfig            `yaml:"low_code"`            // flow 执行相关
	FeatureGate        FeatureGateConfig        `yaml:"feature_gate"`        // 细粒度开关、license
	Database           dbCfg.DatabaseConfig     `yaml:"database"`            // 数据库配置
	Cache              cacheCfg.CacheConfig     `yaml:"cache"`               // 缓存配置
	LogConfig          logCfg.LogConfig         `yaml:"log"`                 // 输出配置
	Audit              AuditConfig              `yaml:"audit"`               // 审计配置
	AI                 agentCfg.AIConfig        `yaml:"ai"`
	Agent              agentCfg.AgentConfig     `yaml:"agent"` // 智能体工具注册/限流等
	Plugin             PluginAggregateConfig    `yaml:"plugin"`
	HTTPSecurity       HTTPSecurityConfig       `yaml:"http_security"`
	Storage            StorageConfig            `yaml:"storage"`
	Tenants            TenantConfig             `yaml:"tenants"`
	Install            InstallConfig            `yaml:"install"`
}

// AuditConfig controls audit persistence and sink behaviour.
type AuditConfig struct {
	PersistToDB         bool                `yaml:"persist_to_db"`
	EnableGORMCallbacks bool                `yaml:"enable_gorm_callbacks"`
	File                AuditFileSinkConfig `yaml:"file"`
}

type AuditFileSinkConfig struct {
	Enable      bool   `yaml:"enable"`
	Dir         string `yaml:"dir"`
	FilePrefix  string `yaml:"file_prefix"`
	MaxSize     int    `yaml:"max_size"`
	MaxBackups  int    `yaml:"max_backups"`
	MaxAge      int    `yaml:"max_age"`
	Compress    bool   `yaml:"compress"`
	UseUTC      bool   `yaml:"use_utc"`
	IncludeMeta bool   `yaml:"include_meta"`
}

const DefaultSystemVersion = "v1.0.0"

func (c *Config) EffectiveSystemVersion() string {
	if c == nil {
		return DefaultSystemVersion
	}
	version := strings.TrimSpace(c.Version)
	if version == "" {
		return DefaultSystemVersion
	}
	return version
}

func GetSystemVersion() string {
	if GlobalConfig == nil {
		return DefaultSystemVersion
	}
	return GlobalConfig.EffectiveSystemVersion()
}

// EffectiveMCPConfig 返回当前应使用的 MCP 配置。
func (c *Config) EffectiveMCPConfig() *mcpCfg.MCPConfig {
	if c == nil {
		return nil
	}
	return c.Agent.MCP
}

type EventConfig struct {
	Bus    EventBusConfig    `yaml:"bus"`
	Fabric EventFabricConfig `yaml:"fabric"`
}

// HTTP服务器配置
type ServerConfig struct {
	Host                string             `yaml:"host"`                  // HTTP 监听主机
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
	RedisDB       int    `yaml:"redis_db"`       // redis 数据库
	DedupeTTLSec  int    `yaml:"dedupe_ttl_sec"` // 幂等缓存过期
}

// QueueConfig 统一队列配置（允许被多个模块引用）
type QueueConfig struct {
	Driver string              `yaml:"driver"` // redis/local/kafka/rabbitmq/nats
	Redis  QueueRedisConfig    `yaml:"redis"`
	Kafka  QueueKafkaConfig    `yaml:"kafka"`
	Rabbit QueueRabbitMQConfig `yaml:"rabbitmq"`
	NATS   QueueNATSConfig     `yaml:"nats"`
}

// QueueRedisConfig 描述 Redis 连接信息
type QueueRedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// QueueKafkaConfig 描述 Kafka 任务驱动连接参数。
type QueueKafkaConfig struct {
	Brokers       []string `yaml:"brokers"`
	TopicPrefix   string   `yaml:"topic_prefix"`
	ConsumerGroup string   `yaml:"consumer_group"`
	PollTimeoutMs int      `yaml:"poll_timeout_ms"`
}

// QueueRabbitMQConfig 描述 RabbitMQ 驱动连接参数。
type QueueRabbitMQConfig struct {
	URL           string `yaml:"url"`
	Exchange      string `yaml:"exchange"`
	QueuePrefix   string `yaml:"queue_prefix"`
	ConsumerTag   string `yaml:"consumer_tag"`
	Prefetch      int    `yaml:"prefetch"`
	PollTimeoutMs int    `yaml:"poll_timeout_ms"`
}

// QueueNATSConfig 描述 NATS 驱动连接参数。
type QueueNATSConfig struct {
	URLs          []string `yaml:"urls"`
	SubjectPrefix string   `yaml:"subject_prefix"`
	QueueGroup    string   `yaml:"queue_group"`
	PollTimeoutMs int      `yaml:"poll_timeout_ms"`
}

// SchedulerConfig 统一的任务调度配置
type SchedulerConfig struct {
	Driver          string `yaml:"driver"`           // builtin/cron
	IntervalSeconds int    `yaml:"interval_seconds"` // 默认 tick
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

// CapabilityRegistryConfig 配置能力目录缓存与事件主题。
type CapabilityRegistryConfig struct {
	RedisPrefix                    string                               `yaml:"redis_prefix"`
	EventTopicPrefix               string                               `yaml:"event_topic_prefix"`
	DefaultRateLimit               CapabilityRegistryRateLimitConfig    `yaml:"default_rate_limit"`
	DefaultHTTPTimeoutSeconds      int                                  `yaml:"default_http_timeout_seconds"`
	AIMultimodalHTTPTimeoutSeconds int                                  `yaml:"ai_multimodal_http_timeout_seconds"`
	Notifications                  CapabilityRegistryNotificationConfig `yaml:"notifications"`
}

// CapabilityRegistryRateLimitConfig 描述同步/Worker 默认限流。
type CapabilityRegistryRateLimitConfig struct {
	Limit         uint64 `yaml:"limit"`
	Burst         uint64 `yaml:"burst"`
	WindowSeconds int    `yaml:"window_seconds"`
}

// CapabilityRegistryNotificationConfig 定义能力目录告警通知。
type CapabilityRegistryNotificationConfig struct {
	IMWebhook        string `yaml:"im_webhook"`
	RetryIntervalSec int    `yaml:"retry_interval_seconds"`
	RetryMaxAttempts int    `yaml:"retry_max_attempts"`
	HTTPTimeoutSec   int    `yaml:"http_timeout_seconds"`
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
	RedisAddr                string                                 `yaml:"redis_addr"`
	RedisPassword            string                                 `yaml:"redis_password"`
	RedisDB                  int                                    `yaml:"redis_db"`
	LockKeyPrefix            string                                 `yaml:"lock_key_prefix"`
	MetricsKeyPrefix         string                                 `yaml:"metrics_key_prefix"`
	DefaultRetentionMonths   int                                    `yaml:"default_retention_months"`
	ProvisioningSLASeconds   int                                    `yaml:"provisioning_sla_seconds"`
	IngestionSLASeconds      int                                    `yaml:"ingestion_sla_seconds"`
	SceneStrategyCatalogPath string                                 `yaml:"scene_strategy_catalog_path"`
	IngestionProcessors      KnowledgeSpaceIngestionProcessorConfig `yaml:"ingestion_processors"`
	EventTopics              KnowledgeSpaceEventTopics              `yaml:"event_topics"`
	Notifications            KnowledgeSpaceNotificationConfig       `yaml:"notifications"`
	VectorStore              KnowledgeSpaceVectorStoreConfig        `yaml:"vector_store"`
	IndexBackends            KnowledgeSpaceIndexBackendConfig       `yaml:"index_backends"`
	Delta                    KnowledgeSpaceDeltaConfig              `yaml:"delta"`
	Reports                  KnowledgeSpaceReportConfig             `yaml:"reports"`
	EventHotfix              KnowledgeSpaceEventHotfixConfig        `yaml:"event_hotfix"`
	Decay                    KnowledgeSpaceDecayConfig              `yaml:"decay"`
	Release                  KnowledgeSpaceReleaseConfig            `yaml:"release"`
}

// KnowledgeSpaceIngestionProcessorConfig 控制入库处理器能力开关（用于部署环境可控启停）。
// 留空则使用运行时自动探测（PATH 中是否存在对应命令）。
type KnowledgeSpaceIngestionProcessorConfig struct {
	// PDF 内嵌文本抽取：依赖 `pdftotext`（poppler-utils）
	PDFTextAvailable *bool `yaml:"pdf_text_available"`
	// OCR Plan B：依赖 `tesseract` + (`pdftoppm` 或 `mutool`)
	OCRAvailable *bool `yaml:"ocr_available"`
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

// KnowledgeSpaceIndexBackendConfig defines storage backends for non-dense indices.
// Values are intentionally explicit so `make db-migrate` can decide whether to create assist tables.
type KnowledgeSpaceIndexBackendConfig struct {
	// Sparse index backend (for `index.sparse`): `postgres_fts` or `external`.
	Sparse string `yaml:"sparse"`
	// Hier index backend (for `index.hier`): `postgres_links` or `external`.
	Hier string `yaml:"hier"`
	// Structured field filtering backend (for `index.structured_fields`): `postgres_jsonb` or `external`.
	StructuredFields string `yaml:"structured_fields"`
	// KG backend (for `index.kg`): `postgres` or `external`.
	KG string `yaml:"kg"`
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

type KnowledgeSpaceEventHotfixConfig struct {
	PoliciesPath        string `yaml:"policies_path"`
	AgentMatrixPath     string `yaml:"agent_weight_matrix_path"`
	ReportPath          string `yaml:"report_path"`
	AggregateReportPath string `yaml:"aggregate_report_path"`
	RetryMax            int    `yaml:"retry_max"`
	ReplayWindowSeconds int    `yaml:"replay_window_seconds"`
}

type KnowledgeSpaceDecayConfig struct {
	ThresholdPath       string `yaml:"threshold_path"`
	ReportPath          string `yaml:"report_path"`
	AggregateReportPath string `yaml:"aggregate_report_path"`
}

type KnowledgeSpaceReleaseConfig struct {
	MatrixPath          string `yaml:"matrix_path"`
	GuardrailsDoc       string `yaml:"guardrails_doc"`
	ReportPath          string `yaml:"report_path"`
	AggregateReportPath string `yaml:"aggregate_report_path"`
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
	LicenseKey                       string `yaml:"license_key"`                          // license 或灰度控制 token
	EnableEventFabric                bool   `yaml:"enable_event_fabric"`                  // 是否启用事件骨干
	EnableWorkflow                   bool   `yaml:"enable_workflow"`                      // 是否启用 Workflow 能力
	EnableKnowledgeSpace             bool   `yaml:"enable_knowledge_space"`               // 是否启用知识空间
	EnableMediaPlatform              bool   `yaml:"enable_media_platform"`                // 是否启用平台 Media 能力
	EnableExperimentalFeatures       bool   `yaml:"enable_experimental_features"`         // 是否开启实验特性
	EnableSaaSSignup                 bool   `yaml:"enable_saas_signup"`                   // 是否开放 SaaS 新租户注册
	EnableSaaSSignupVerificationCode bool   `yaml:"enable_saas_signup_verification_code"` // SaaS 注册是否要求邮箱/手机验证码
}

// Load 加载配置文件并合并环境变量
func Load(configPath string) (*Config, error) {
	// 1. 加载默认配置
	cfg := GetDefaults()

	// 1.1 预加载 .env（仅在当前进程变量未设置时写入）
	loadDotEnvCandidates(configPath)

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

func loadDotEnvCandidates(configPath string) {
	// 本地开发模式统一只认 backend/.env，避免多来源导致覆盖混乱。
	envMode := strings.ToLower(strings.TrimSpace(os.Getenv("POWERX_ENV")))
	if envMode != "" && envMode != "dev" {
		return
	}
	candidates := make([]string, 0, 3)
	if configPath != "" {
		if absCfg, err := filepath.Abs(configPath); err == nil {
			backendDir := filepath.Dir(filepath.Dir(absCfg))
			candidates = append(candidates, filepath.Join(backendDir, ".env"))
		}
	}
	if p := findConfigPath("backend/.env"); p != "" {
		candidates = append(candidates, p)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "backend", ".env"))
		candidates = append(candidates, filepath.Join(wd, ".env"))
	}

	seen := map[string]struct{}{}
	for _, p := range candidates {
		if strings.TrimSpace(p) == "" {
			continue
		}
		absPath := p
		if !filepath.IsAbs(p) {
			if wd, err := os.Getwd(); err == nil {
				absPath = filepath.Join(wd, p)
			}
		}
		absPath = filepath.Clean(absPath)
		if _, ok := seen[absPath]; ok {
			continue
		}
		seen[absPath] = struct{}{}
		_ = loadDotEnvFile(absPath)
	}
}

func loadDotEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		val = strings.Trim(val, `"'`)
		// 保留已存在环境变量优先级；.env 仅补缺省
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

// loadFromEnv 从环境变量加载配置
func loadFromEnv(cfg *Config) {
	// Server配置
	if host := os.Getenv("POWERX_BACKEND_HOST"); host != "" {
		cfg.Server.Host = strings.TrimSpace(host)
	} else if host := os.Getenv("CORE_X_SERVER_HOST"); host != "" {
		cfg.Server.Host = strings.TrimSpace(host)
	}
	if port := os.Getenv("POWERX_BACKEND_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Server.Port = p
		}
	} else if port := os.Getenv("CORE_X_SERVER_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Server.Port = p
		}
	}
	if mode := os.Getenv("CORE_X_SERVER_MODE"); mode != "" {
		cfg.Server.Mode = mode
	}
	if port := os.Getenv("POWERX_GRPC_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Server.GRPC.Port = p
		}
	} else if port := os.Getenv("CORE_X_SERVER_GRPC_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Server.GRPC.Port = p
		}
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
		cfg.Event.Bus.Type = busType
	}
	if redisAddr := os.Getenv("CORE_X_EVENT_BUS_REDIS_ADDR"); redisAddr != "" {
		cfg.Event.Bus.RedisAddr = redisAddr
	}
	if redisPassword := os.Getenv("CORE_X_EVENT_BUS_REDIS_PASSWORD"); redisPassword != "" {
		cfg.Event.Bus.RedisPassword = redisPassword
	}
	if redisDB := os.Getenv("CORE_X_EVENT_BUS_REDIS_DB"); redisDB != "" {
		if v, err := strconv.Atoi(redisDB); err == nil {
			cfg.Event.Bus.RedisDB = v
		}
	}
	if ttl := os.Getenv("CORE_X_EVENT_BUS_DEDUPE_TTL_SEC"); ttl != "" {
		if t, err := strconv.Atoi(ttl); err == nil {
			cfg.Event.Bus.DedupeTTLSec = t
		}
	}

	// Capability Registry 配置
	if v := os.Getenv("CORE_X_CAPABILITY_REGISTRY_REDIS_PREFIX"); v != "" {
		cfg.CapabilityRegistry.RedisPrefix = v
	}
	if v := os.Getenv("CORE_X_CAPABILITY_REGISTRY_EVENT_TOPIC_PREFIX"); v != "" {
		cfg.CapabilityRegistry.EventTopicPrefix = v
	}
	if v := os.Getenv("CORE_X_CAPABILITY_REGISTRY_RATE_LIMIT_LIMIT"); v != "" {
		if limit, err := strconv.ParseUint(v, 10, 64); err == nil && limit > 0 {
			cfg.CapabilityRegistry.DefaultRateLimit.Limit = limit
		}
	}
	if v := os.Getenv("CORE_X_CAPABILITY_REGISTRY_RATE_LIMIT_BURST"); v != "" {
		if burst, err := strconv.ParseUint(v, 10, 64); err == nil && burst > 0 {
			cfg.CapabilityRegistry.DefaultRateLimit.Burst = burst
		}
	}
	if v := os.Getenv("CORE_X_CAPABILITY_REGISTRY_RATE_LIMIT_WINDOW"); v != "" {
		if window, err := strconv.Atoi(v); err == nil && window > 0 {
			cfg.CapabilityRegistry.DefaultRateLimit.WindowSeconds = window
		}
	}
	if v := os.Getenv("CORE_X_CAPABILITY_REGISTRY_DEFAULT_HTTP_TIMEOUT_SECONDS"); v != "" {
		if timeout, err := strconv.Atoi(v); err == nil && timeout > 0 {
			cfg.CapabilityRegistry.DefaultHTTPTimeoutSeconds = timeout
		}
	}
	if v := os.Getenv("CORE_X_CAPABILITY_REGISTRY_AI_MULTIMODAL_HTTP_TIMEOUT_SECONDS"); v != "" {
		if timeout, err := strconv.Atoi(v); err == nil && timeout > 0 {
			cfg.CapabilityRegistry.AIMultimodalHTTPTimeoutSeconds = timeout
		}
	}

	// EventFabric 配置
	if v := os.Getenv("CORE_X_EVENT_FABRIC_ACK_TIMEOUT_SEC"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.Event.Fabric.AckTimeoutSeconds = t
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_DEFAULT_MAX_RETRY"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.Event.Fabric.DefaultMaxRetry = t
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_REDIS_ADDR"); v != "" {
		cfg.Event.Fabric.RedisAddr = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_REDIS_PASSWORD"); v != "" {
		cfg.Event.Fabric.RedisPassword = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_REDIS_DB"); v != "" {
		if dbIdx, err := strconv.Atoi(v); err == nil {
			cfg.Event.Fabric.RedisDB = dbIdx
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_RETRY_KEY_PREFIX"); v != "" {
		cfg.Event.Fabric.RetryKeyPrefix = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_REPLAY_KEY_PREFIX"); v != "" {
		cfg.Event.Fabric.ReplayKeyPrefix = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_SCHEDULER_INTERVAL"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.Event.Fabric.SchedulerInterval = t
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_REQUIRE_TLS"); v != "" {
		cfg.Event.Fabric.Security.RequireTLS = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_SIGNATURE_SECRET"); v != "" {
		cfg.Event.Fabric.Security.SignatureSecret = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_SIGNATURE_HEADER"); v != "" {
		cfg.Event.Fabric.Security.SignatureHeader = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_TIMESTAMP_HEADER"); v != "" {
		cfg.Event.Fabric.Security.TimestampHeader = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_SIGNATURE_KEY_ID"); v != "" {
		cfg.Event.Fabric.Security.SignatureKeyID = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_ALLOWED_SKEW_SEC"); v != "" {
		if skew, err := strconv.Atoi(v); err == nil {
			cfg.Event.Fabric.Security.AllowedClockSkewSeconds = skew
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_CACHE_TTL"); v != "" {
		if ttl, err := strconv.Atoi(v); err == nil && ttl > 0 {
			cfg.Event.Fabric.Authorization.CacheTTLSeconds = ttl
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_LOCAL_CACHE_TTL"); v != "" {
		if ttl, err := strconv.Atoi(v); err == nil && ttl > 0 {
			cfg.Event.Fabric.Authorization.LocalCacheTTLSeconds = ttl
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_REDIS_ADDR"); v != "" {
		cfg.Event.Fabric.Authorization.RedisAddr = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_REDIS_PASSWORD"); v != "" {
		cfg.Event.Fabric.Authorization.RedisPassword = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_REDIS_DB"); v != "" {
		if dbIdx, err := strconv.Atoi(v); err == nil && dbIdx >= 0 {
			cfg.Event.Fabric.Authorization.RedisDB = dbIdx
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_CACHE_INVALIDATE_CHANNEL"); v != "" {
		cfg.Event.Fabric.Authorization.CacheInvalidateChannel = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_CHALLENGE_SLA"); v != "" {
		if sla, err := strconv.Atoi(v); err == nil && sla > 0 {
			cfg.Event.Fabric.Authorization.ChallengeSLASeconds = sla
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_CHALLENGE_TOPIC"); v != "" {
		cfg.Event.Fabric.Authorization.ChallengeTopic = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_CHALLENGE_CONSUMER_GROUP"); v != "" {
		cfg.Event.Fabric.Authorization.ChallengeConsumerGroup = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_AUDIT_RETENTION_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days > 0 {
			cfg.Event.Fabric.Authorization.AuditRetentionDays = days
		}
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_AUDIT_ARCHIVE_BUCKET"); v != "" {
		cfg.Event.Fabric.Authorization.AuditArchiveBucket = v
	}
	if v := os.Getenv("CORE_X_EVENT_FABRIC_AUTHZ_AUDIT_ARCHIVE_PREFIX"); v != "" {
		cfg.Event.Fabric.Authorization.AuditArchivePrefix = v
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
	if v := os.Getenv("CORE_X_LOG_FILE_ENABLE"); v != "" {
		cfg.LogConfig.File.Enable = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_LOG_FILE_INFO_PATH"); v != "" {
		cfg.LogConfig.File.InfoFilePath = strings.TrimSpace(v)
	}
	if v := os.Getenv("CORE_X_LOG_FILE_ERROR_PATH"); v != "" {
		cfg.LogConfig.File.ErrorFilePath = strings.TrimSpace(v)
	}
	if v := os.Getenv("CORE_X_LOG_AGENT_DEBUG_DIR"); v != "" {
		cfg.LogConfig.AgentDebug.Dir = strings.TrimSpace(v)
	}
	if v := os.Getenv("CORE_X_LOG_CONSOLE"); v != "" {
		cfg.LogConfig.Console = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_LOG_RETENTION_ENABLED"); v != "" {
		cfg.LogConfig.Retention.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := strings.TrimSpace(os.Getenv("CORE_X_LOG_RETENTION_CRON")); v != "" {
		cfg.LogConfig.Retention.Cron = v
	}
	if v := strings.TrimSpace(os.Getenv("CORE_X_LOG_RETENTION_TIMEZONE")); v != "" {
		cfg.LogConfig.Retention.Timezone = v
	}
	if v := os.Getenv("CORE_X_LOG_RETENTION_DEFAULT_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.LogConfig.Retention.DefaultRetentionDays = n
		}
	}
	if v := os.Getenv("CORE_X_LOG_RETENTION_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.LogConfig.Retention.BatchSize = n
		}
	}
	if v := os.Getenv("CORE_X_LOG_RETENTION_MAX_DELETE_ROWS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.LogConfig.Retention.MaxDeleteRowsPerRun = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("CORE_X_LOG_RETENTION_FILE_PATHS")); v != "" {
		raw := strings.Split(v, ",")
		paths := make([]string, 0, len(raw))
		for i := range raw {
			if p := strings.TrimSpace(raw[i]); p != "" {
				paths = append(paths, p)
			}
		}
		if len(paths) > 0 {
			cfg.LogConfig.Retention.FilePaths = paths
		}
	}

	if v := os.Getenv("CORE_X_AUDIT_PERSIST_TO_DB"); v != "" {
		cfg.Audit.PersistToDB = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_AUDIT_ENABLE_GORM_CALLBACKS"); v != "" {
		cfg.Audit.EnableGORMCallbacks = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_AUDIT_FILE_ENABLE"); v != "" {
		cfg.Audit.File.Enable = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_AUDIT_FILE_DIR"); v != "" {
		cfg.Audit.File.Dir = v
	}
	if v := os.Getenv("CORE_X_AUDIT_FILE_PREFIX"); v != "" {
		cfg.Audit.File.FilePrefix = v
	}
	if v := os.Getenv("CORE_X_AUDIT_FILE_MAX_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Audit.File.MaxSize = n
		}
	}
	if v := os.Getenv("CORE_X_AUDIT_FILE_MAX_BACKUPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.Audit.File.MaxBackups = n
		}
	}
	if v := os.Getenv("CORE_X_AUDIT_FILE_MAX_AGE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.Audit.File.MaxAge = n
		}
	}
	if v := os.Getenv("CORE_X_AUDIT_FILE_COMPRESS"); v != "" {
		cfg.Audit.File.Compress = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_AUDIT_FILE_USE_UTC"); v != "" {
		cfg.Audit.File.UseUTC = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_AUDIT_FILE_INCLUDE_META"); v != "" {
		cfg.Audit.File.IncludeMeta = strings.EqualFold(v, "true") || v == "1"
	}

	// FeatureGate配置
	if license := os.Getenv("CORE_X_FEATURE_GATE_LICENSE_KEY"); license != "" {
		cfg.FeatureGate.LicenseKey = license
	}
	if v := os.Getenv("CORE_X_FEATURE_GATE_ENABLE_SAAS_SIGNUP"); v != "" {
		cfg.FeatureGate.EnableSaaSSignup = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_FEATURE_GATE_ENABLE_SAAS_SIGNUP_VERIFICATION_CODE"); v != "" {
		cfg.FeatureGate.EnableSaaSSignupVerificationCode = strings.EqualFold(v, "true") || v == "1"
	}

	// Plugin Release 配置
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ENABLE_LOCAL_INSTALL"); v != "" {
		cfg.Plugin.Release.FeatureFlags.EnableLocalInstall = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ENABLE_PIPELINE_DEPLOYMENT"); v != "" {
		cfg.Plugin.Release.FeatureFlags.EnablePipelineDeployment = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ENABLE_OFFLINE_DISTRIBUTION"); v != "" {
		cfg.Plugin.Release.FeatureFlags.EnableOfflineDistribution = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_SESSION_TTL_MINUTES"); v != "" {
		if ttl, err := strconv.Atoi(v); err == nil && ttl > 0 {
			cfg.Plugin.Release.LocalInstall.SessionTTLMinutes = ttl
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_MAX_ARTIFACT_SIZE_MB"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			cfg.Plugin.Release.LocalInstall.MaxArtifactSizeMB = size
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_APPROVAL_SLA_HOURS"); v != "" {
		if sla, err := strconv.Atoi(v); err == nil && sla > 0 {
			cfg.Plugin.Release.Pipeline.ApprovalSLAHours = sla
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_MAX_PARALLEL_RELEASES"); v != "" {
		if maxR, err := strconv.Atoi(v); err == nil && maxR > 0 {
			cfg.Plugin.Release.Pipeline.MaxParallelReleases = maxR
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_DEFAULT_ROLLBACK_NOTICE_MINUTES"); v != "" {
		if minutes, err := strconv.Atoi(v); err == nil && minutes > 0 {
			cfg.Plugin.Release.Pipeline.DefaultRollbackNotice = minutes
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_CANARY_ROLLBACK_TIMEOUT_SECONDS"); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
			cfg.Plugin.Release.Canary.RollbackTimeoutSeconds = seconds
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_CANARY_DEFAULT_BATCH_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			cfg.Plugin.Release.Canary.DefaultBatchSize = size
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_CANARY_MAX_BATCHES"); v != "" {
		if count, err := strconv.Atoi(v); err == nil && count > 0 {
			cfg.Plugin.Release.Canary.MaxBatches = count
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_OFFLINE_BUCKET"); v != "" {
		cfg.Plugin.Release.Distribution.OfflineBucket = v
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_OFFLINE_PREFIX"); v != "" {
		cfg.Plugin.Release.Distribution.OfflinePrefix = v
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ESCALATION_THRESHOLD"); v != "" {
		if threshold, err := strconv.Atoi(v); err == nil && threshold > 0 {
			cfg.Plugin.Release.Distribution.EscalationThreshold = threshold
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ARTIFACT_RETENTION_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days > 0 {
			cfg.Plugin.Release.Distribution.ArtifactRetentionDays = days
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_DASHBOARD_UID"); v != "" {
		cfg.Plugin.Release.Observability.DashboardUID = v
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_ALERT_RULE_PREFIX"); v != "" {
		cfg.Plugin.Release.Observability.AlertRulePrefix = v
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_KPI_ROLLBACK_SECONDS"); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
			cfg.Plugin.Release.Observability.KPITargets.CanRollbackWithinSeconds = seconds
		}
	}
	if v := os.Getenv("CORE_X_PLUGIN_RELEASE_KPI_HOTLOAD_P95_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			cfg.Plugin.Release.Observability.KPITargets.HotloadLatencyP95Ms = ms
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
	if busType := os.Getenv("EVENT_BUS_TYPE"); busType != "" && cfg.Event.Bus.Type == "local" {
		cfg.Event.Bus.Type = busType
	}
}
