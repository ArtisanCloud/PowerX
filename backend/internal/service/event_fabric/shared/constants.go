package shared

import "time"

const (
	// DomainName 统一事件骨干的域标识，用于日志、监控与错误码前缀。
	DomainName = "event_fabric"

	// ContextTenantKey 标识上下文里租户信息的键名。
	ContextTenantKey = "tenant_uuid"

	// ContextSubscriberKey 标识上下文中订阅者的键名。
	ContextSubscriberKey = "subscriber_id"

	// ContextTopicsKey 用于传递订阅过滤的 Topic 列表。
	ContextTopicsKey = "topics"

	// ContextCompatibilityMode 标识订阅者声明的版本兼容策略。
	ContextCompatibilityMode = "compatibility_mode"

	// ContextAcceptedVersions 存储订阅者声明支持的事件版本列表。
	ContextAcceptedVersions = "accepted_versions"

	// DefaultAckTimeout 与规格保持一致，默认 30s。
	DefaultAckTimeout = 30 * time.Second

	// DefaultMaxRetry 默认最大重试次数 5 次。
	DefaultMaxRetry = 5

	// RetryKeyPrefix / ReplayKeyPrefix 为 Redis key 的默认前缀。
	RetryKeyPrefix  = "event_fabric:retry"
	ReplayKeyPrefix = "event_fabric:replay"
)

const (
	// DeliveryStatusPending 表示等待初次投递。
	DeliveryStatusPending = "pending"

	// DeliveryStatusDelivering 表示正在投递或重试中。
	DeliveryStatusDelivering = "delivering"

	// DeliveryStatusSucceeded 表示订阅者已 Ack。
	DeliveryStatusSucceeded = "succeeded"

	// DeliveryStatusFailed 表示进入死信或彻底失败。
	DeliveryStatusFailed = "failed"
)
