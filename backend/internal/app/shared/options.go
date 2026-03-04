package shared

// internal/app/shared/options.go

import (
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/auth"
	security "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/security"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	milvuscfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/milvus"
	pgvectorcfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pgvector"
	pineconecfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pinecone"
)

type DepsOptions struct {
	AuthUser     auth.AuthOptions      // 给用户端的 Audience
	AuthCustomer auth.AuthOptions      // 给客户/插件端的 Audience
	Audit        auditsvc.AuditOptions // 批量大小、等待等
	Storage      mediasvc.StorageOptions
	Queue        QueueOptions
	// 以后需要别的也放在这里（如默认租户、开关等）
	EventFabric        EventFabricOptions
	Workflow           WorkflowOptions
	IntegrationGateway IntegrationGatewayOptions
	CapabilityRegistry CapabilityRegistryOptions
	AgentLifecycle     AgentLifecycleOptions
	KnowledgeSpace     KnowledgeSpaceOptions
	DevHotload         DevHotloadOptions
	PluginRelease      PluginReleaseOptions
	PluginBootstrap    PluginBootstrapOptions
	PluginDebug        PluginDebugOptions
	Server             ServerOptions
}

// QueueOptions 描述任务驱动的配置入口。
type QueueOptions struct {
	Driver string
	Kafka  QueueKafkaOptions
	Rabbit QueueRabbitMQOptions
	NATS   QueueNATSOptions
}

// QueueKafkaOptions 描述 Kafka 驱动连接参数。
type QueueKafkaOptions struct {
	Brokers       []string
	TopicPrefix   string
	ConsumerGroup string
	PollTimeoutMs int
}

// QueueRabbitMQOptions 描述 RabbitMQ 驱动连接参数。
type QueueRabbitMQOptions struct {
	URL           string
	Exchange      string
	QueuePrefix   string
	ConsumerTag   string
	Prefetch      int
	PollTimeoutMs int
}

// QueueNATSOptions 描述 NATS 驱动连接参数。
type QueueNATSOptions struct {
	URLs          []string
	SubjectPrefix string
	QueueGroup    string
	PollTimeoutMs int
}

type ServerOptions struct {
	GRPC GRPCServerOptions
}

type GRPCServerOptions struct {
	Host string
	Port int
}

// EventFabricOptions 描述事件骨干依赖的运行配置。
type EventFabricOptions struct {
	AckTimeoutSeconds int
	DefaultMaxRetry   int
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	RetryKeyPrefix    string
	ReplayKeyPrefix   string
	SchedulerInterval int
	Security          security.Config
	Authorization     EventFabricAuthorizationOptions
}

// EventFabricAuthorizationOptions 描述授权域所需运行参数。
type EventFabricAuthorizationOptions struct {
	CacheTTLSeconds             int
	LocalCacheTTLSeconds        int
	RedisAddr                   string
	RedisPassword               string
	RedisDB                     int
	CacheInvalidateChannel      string
	ChallengeSLASeconds         int
	ChallengeTopic              string
	ChallengeConsumerGroup      string
	AlertTopic                  string
	RateLimitPrefix             string
	TimeoutSweepIntervalSeconds int
	AuditRetentionDays          int
	AuditArchiveBucket          string
	AuditArchivePrefix          string
	Secrets                     EventFabricAuthorizationSecretsOptions
}

// EventFabricAuthorizationSecretsOptions 描述 KMS 相关参数。
type EventFabricAuthorizationSecretsOptions struct {
	Provider                string
	KeyID                   string
	RotationIntervalSeconds int
	CacheTTLSeconds         int
}

// WorkflowOptions 描述工作流域的运行配置（占位，后续完善）。
type WorkflowOptions struct {
	RetryKeyPrefix string
}

// IntegrationGatewayOptions 描述集成网关所需的基础运行配置。
type IntegrationGatewayOptions struct {
	RateLimitPrefix  string
	RedisAddr        string
	RedisPassword    string
	RedisDB          int
	DefaultRateLimit IntegrationGatewayRateLimitOptions
	EventTopics      IntegrationGatewayEventTopicsOptions
}

// IntegrationGatewayRateLimitOptions 表示默认限流策略。
type IntegrationGatewayRateLimitOptions struct {
	Limit         uint64
	Burst         uint64
	WindowSeconds int
	Scope         string
}

// IntegrationGatewayEventTopicsOptions 包含事件主题名称。
type IntegrationGatewayEventTopicsOptions struct {
	Created             string
	Updated             string
	InvocationSucceeded string
	InvocationFailed    string
}

// AgentLifecycleOptions 描述代理生命周期模块的共享依赖。
type AgentLifecycleOptions struct {
	RedisAddr                string
	RedisPassword            string
	RedisDB                  int
	CapacityKeyPrefix        string
	HealthKeyPrefix          string
	DefaultCapacityInstances int
	EventTopics              AgentLifecycleEventTopicsOptions
	Notifications            AgentLifecycleNotificationOptions
	StateBusTopics           AgentLifecycleStateBusTopicsOptions
	ShareReviewInterval      time.Duration
}

// AgentLifecycleEventTopicsOptions 定义事件主题前缀。
type AgentLifecycleEventTopicsOptions struct {
	LifecyclePrefix string
	HealthPrefix    string
}

// AgentLifecycleStateBusTopicsOptions 定义 StateBus 主题。
type AgentLifecycleStateBusTopicsOptions struct {
	Lifecycle string
	Health    string
}

// AgentLifecycleNotificationOptions 定义通知发送行为。
type AgentLifecycleNotificationOptions struct {
	IMWebhook        string
	RetryInterval    time.Duration
	RetryMaxAttempts int
	HTTPTimeout      time.Duration
}

// KnowledgeSpaceOptions 描述知识空间域依赖。
type KnowledgeSpaceOptions struct {
	RedisAddr                string
	RedisPassword            string
	RedisDB                  int
	LockKeyPrefix            string
	MetricsKeyPrefix         string
	DefaultRetentionMonths   int
	ProvisioningSLA          time.Duration
	IngestionSLA             time.Duration
	SceneStrategyCatalogPath string
	IngestionProcessors      KnowledgeSpaceIngestionProcessorOptions
	EventTopics              KnowledgeSpaceEventTopicsOptions
	Notifications            KnowledgeSpaceNotificationOptions
	VectorStore              KnowledgeSpaceVectorStoreOptions
	Delta                    KnowledgeSpaceDeltaOptions
	Reports                  KnowledgeSpaceReportOptions
	EventHotfix              KnowledgeSpaceEventHotfixOptions
	Decay                    KnowledgeSpaceDecayOptions
	Release                  KnowledgeSpaceReleaseOptions
}

// KnowledgeSpaceIngestionProcessorOptions 控制入库处理器能力开关（nil 表示自动探测）。
type KnowledgeSpaceIngestionProcessorOptions struct {
	PDFTextAvailable *bool
	OCRAvailable     *bool
}

type KnowledgeSpaceEventTopicsOptions struct {
	Provisioning string
	Ingestion    string
	Fusion       string
	Feedback     string
}

type KnowledgeSpaceNotificationOptions struct {
	IMWebhook        string
	RetryInterval    time.Duration
	RetryMaxAttempts int
	HTTPTimeout      time.Duration
}

// CapabilityRegistryOptions 描述能力目录相关配置。
type CapabilityRegistryOptions struct {
	Notifications CapabilityRegistryNotificationOptions
}

// CapabilityRegistryNotificationOptions 定义告警通知。
type CapabilityRegistryNotificationOptions struct {
	IMWebhook        string
	RetryInterval    time.Duration
	RetryMaxAttempts int
	HTTPTimeout      time.Duration
}

type KnowledgeSpaceVectorStoreOptions struct {
	Driver   string
	PGVector pgvectorcfg.Config
	Milvus   milvuscfg.Config
	Pinecone pineconecfg.Config
}

type KnowledgeSpaceDeltaOptions struct {
	SourcesConfig        string
	PartialReleaseConfig string
	ReportPath           string
	AggregateReportPath  string
	SLAMinutes           int
	ApprovalMinutes      int
	DefaultDiffAccuracy  float64
}

type KnowledgeSpaceReportOptions struct {
	FeedbackPath string
	QABridgePath string
}

type KnowledgeSpaceEventHotfixOptions struct {
	PoliciesPath        string
	AgentMatrixPath     string
	ReportPath          string
	AggregateReportPath string
	RetryMax            int
	ReplayWindow        time.Duration
}

type KnowledgeSpaceDecayOptions struct {
	ThresholdPath       string
	ReportPath          string
	AggregateReportPath string
}

type KnowledgeSpaceReleaseOptions struct {
	MatrixPath          string
	GuardrailsDoc       string
	ReportPath          string
	AggregateReportPath string
}

// PluginReleaseOptions 暴露插件发布模块所需运行参数。
type PluginReleaseOptions struct {
	FeatureFlags  PluginReleaseFeatureFlagsOptions
	LocalInstall  PluginReleaseLocalInstallOptions
	Pipeline      PluginReleasePipelineOptions
	Canary        PluginReleaseCanaryOptions
	Distribution  PluginReleaseDistributionOptions
	Observability PluginReleaseObservabilityOptions
}

type PluginReleaseFeatureFlagsOptions struct {
	EnableLocalInstall        bool
	EnablePipelineDeployment  bool
	EnableOfflineDistribution bool
}

type PluginReleaseLocalInstallOptions struct {
	SessionTTL        time.Duration
	MaxArtifactSizeMB int
}

type PluginReleasePipelineOptions struct {
	ApprovalSLA           time.Duration
	MaxParallelReleases   int
	DefaultRollbackNotice time.Duration
}

type PluginReleaseCanaryOptions struct {
	RollbackTimeout  time.Duration
	DefaultBatchSize int
	MaxBatches       int
}

type PluginReleaseDistributionOptions struct {
	OfflineBucket       string
	OfflinePrefix       string
	EscalationThreshold int
	ArtifactRetention   time.Duration
}

type PluginReleaseObservabilityOptions struct {
	DashboardUID    string
	AlertRulePrefix string
	KPITargets      PluginReleaseKPITargetsOptions
}

type PluginReleaseKPITargetsOptions struct {
	CanRollbackWithin time.Duration
	HotloadLatencyP95 time.Duration
}

// DevHotloadOptions exposes Dev API gateway runtime configuration.
type DevHotloadOptions struct {
	FeatureFlags  DevHotloadFeatureFlagsOptions
	Sessions      DevHotloadSessionOptions
	Sandbox       DevHotloadSandboxOptions
	Security      DevHotloadSecurityOptions
	Observability DevHotloadObservabilityOptions
}

type DevHotloadFeatureFlagsOptions struct {
	Enabled          bool
	GatewayFlag      string
	SessionAuditFlag string
}

type DevHotloadSessionOptions struct {
	TTL             time.Duration
	MaxConcurrent   int
	CleanupInterval time.Duration
}

type DevHotloadSandboxOptions struct {
	Image          string
	MaxCPUPercent  int
	MaxMemoryMB    int
	WatchFileLimit int
}

type DevHotloadSecurityOptions struct {
	RequireMTLS     bool
	AllowedSubjects []string
	PATHeader       string
	TokenTTL        time.Duration
	TokenSecret     []byte
	TokenIssuer     string
	TokenAudience   string
	TokenPlatforms  []string
	TokenRoles      []string
	ImpersonateRoot bool
}

type DevHotloadObservabilityOptions struct {
	MetricsNamespace string
	SSEBufferSize    int
	AuditTopic       string
}

// PluginDebugOptions configures plugin debug utilities.
type PluginDebugOptions struct {
	Component     string
	HostSimulator PluginDebugHostOptions
	Reports       PluginDebugReportOptions
	TicketBridge  PluginDebugTicketBridgeOptions
	Sandbox       PluginDebugSandboxOptions
}

// PluginDebugHostOptions toggles host simulator behaviour.
type PluginDebugHostOptions struct {
	Enabled     bool
	FeatureFlag string
	ConfigPath  string
}

// PluginDebugReportOptions describes report template/masking.
type PluginDebugReportOptions struct {
	TemplatePath     string
	MaskingRulesPath string
	FallbackLogBase  string
}

// PluginDebugTicketBridgeOptions configure ticket escalation.
type PluginDebugTicketBridgeOptions struct {
	Provider string
	Endpoint string
	Project  string
}

// PluginDebugSandboxOptions controls sandbox orchestration.
type PluginDebugSandboxOptions struct {
	Enabled       bool
	FeatureFlag   string
	DataSuitePath string
}

// PluginBootstrapOptions configures template registry + validation defaults.
type PluginBootstrapOptions struct {
	TemplatesPath   string
	DefaultTemplate string
	AllowHosts      []string
}
