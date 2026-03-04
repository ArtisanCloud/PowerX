// Package event_bus 提供事件总线接口和实现
package event_bus

import (
	"context"
	"time"
)

// Event 事件结构
type Event struct {
	Name       string          `json:"name"`        // 事件名称
	Payload    interface{}     `json:"payload"`     // 事件数据
	Ctx        context.Context `json:"-"`           // 上下文（不序列化）
	ID         string          `json:"id"`          // 事件ID（用于幂等）
	TraceID    string          `json:"trace_id"`    // 追踪ID
	TenantUUID string          `json:"tenant_uuid"` // 租户 UUID
}

// Handler 事件处理器函数类型
type Handler func(Event) error

// EventBus 事件总线接口（保留原有 API 以兼容老模块）。
type EventBus interface {
	// Subscribe 订阅事件，返回取消订阅函数
	Subscribe(eventType string, handler Handler) (unsubscribe func())

	// Publish 发布事件
	Publish(eventType string, payload interface{}, ctx context.Context)

	// Close 关闭事件总线
	Close() error
}

// RetryItem 描述待重试或延迟投递的流水。
type RetryItem struct {
	EventID      string            `json:"event_id"`
	EnvelopeUUID string            `json:"envelope_uuid"`
	SubscriberID string            `json:"subscriber_id"`
	TenantKey    string            `json:"tenant_key"`
	Attempt      int               `json:"attempt"`
	ExecuteAt    time.Time         `json:"execute_at"`
	Backoff      time.Duration     `json:"backoff"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// DeliveryLease 表示某个订阅者的并发租约，用于 Ack/Nack 超时控制。
type DeliveryLease struct {
	LeaseID       string
	TenantKey     string
	SubscriberID  string
	AckTimeout    time.Duration
	MaxConcurrent int
}

// ReliableQueue 定义高可靠投递所需的幂等/重试能力。
type ReliableQueue interface {
	// ScheduleRetry 将事件推入带时间戳的重试队列，score=ExecuteAt。
	ScheduleRetry(ctx context.Context, item RetryItem) error

	// PopDueRetries 拉取到期的重试任务，并从队列中移除。
	PopDueRetries(ctx context.Context, tenantKey string, now time.Time, limit int) ([]RetryItem, error)

	// RemoveRetry 从队列中移除指定任务（用于 Ack 成功后清理）。
	RemoveRetry(ctx context.Context, item RetryItem) error

	// AcquireLease 为订阅者申请并发租约，返回是否成功。
	AcquireLease(ctx context.Context, lease DeliveryLease) (bool, error)

	// ReleaseLease 释放租约（Ack/Nack 完成或超时清理）。
	ReleaseLease(ctx context.Context, lease DeliveryLease) error
}

// QueueDriverType 表示任务队列驱动类型。
type QueueDriverType string

const (
	QueueDriverRedis    QueueDriverType = "redis"
	QueueDriverKafka    QueueDriverType = "kafka"
	QueueDriverRabbitMQ QueueDriverType = "rabbitmq"
	QueueDriverNATS     QueueDriverType = "nats"
	QueueDriverDatabase QueueDriverType = "database"
	QueueDriverMemory   QueueDriverType = "memory"
)

// QueueDriverCapability 描述驱动能力，用于编排层在运行时做路由或降级决策。
type QueueDriverCapability struct {
	SupportsBlockingDequeue bool
	SupportsDelayQueue      bool
	SupportsLease           bool
	SupportsConsumerGroup   bool
}

// TaskMessage 是任务驱动层的统一消息信封。
type TaskMessage struct {
	ID           string
	TenantKey    string
	SubscriberID string
	Topic        string
	Payload      []byte
	Headers      map[string]string
	Attempt      int
	TraceID      string
	VisibleAt    time.Time
	Metadata     map[string]string
}

// DequeueRequest 描述一次任务拉取请求。
type DequeueRequest struct {
	TenantKey    string
	SubscriberID string
	MaxItems     int
	WaitTimeout  time.Duration
}

// AckRequest 描述一次确认消费请求。
type AckRequest struct {
	TenantKey    string
	SubscriberID string
	MessageID    string
}

// NackRequest 描述一次拒绝消费请求。
type NackRequest struct {
	TenantKey    string
	SubscriberID string
	MessageID    string
	Reason       string
	RetryAt      time.Time
	Metadata     map[string]string
}

// RetryRequest 描述主动重排队请求。
type RetryRequest struct {
	Message TaskMessage
	RetryAt time.Time
	Reason  string
}

// TaskDriver 定义统一任务驱动接口（enqueue/dequeue/ack/nack/retry）。
// T065 先建立契约，后续任务再接入具体实现与运行时装配。
type TaskDriver interface {
	Type() QueueDriverType
	Capability() QueueDriverCapability
	Enqueue(ctx context.Context, message TaskMessage) error
	Dequeue(ctx context.Context, request DequeueRequest) ([]TaskMessage, error)
	Ack(ctx context.Context, request AckRequest) error
	Nack(ctx context.Context, request NackRequest) error
	Retry(ctx context.Context, request RetryRequest) error
}
