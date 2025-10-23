package shared

// internal/app/shared/options.go

import (
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/auth"
	security "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/security"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
)

type DepsOptions struct {
	AuthUser     auth.AuthOptions      // 给用户端的 Audience
	AuthCustomer auth.AuthOptions      // 给客户/插件端的 Audience
	Audit        auditsvc.AuditOptions // 批量大小、等待等
	Storage      mediasvc.StorageOptions
	// 以后需要别的也放在这里（如默认租户、开关等）
	EventFabric        EventFabricOptions
	Workflow           WorkflowOptions
	IntegrationGateway IntegrationGatewayOptions
	AgentLifecycle     AgentLifecycleOptions
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
}

// AgentLifecycleEventTopicsOptions 定义事件主题前缀。
type AgentLifecycleEventTopicsOptions struct {
	LifecyclePrefix string
	HealthPrefix    string
}

// AgentLifecycleNotificationOptions 定义通知发送行为。
type AgentLifecycleNotificationOptions struct {
	IMWebhook        string
	RetryInterval    time.Duration
	RetryMaxAttempts int
	HTTPTimeout      time.Duration
}
