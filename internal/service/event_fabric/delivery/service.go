package delivery

import (
	"context"
	"time"
)

// PublishRequest 统一发布事件所需字段。
type PublishRequest struct {
	TenantID       string
	Topic          string
	EventID        string
	TraceID        string
	Version        string
	Payload        []byte
	PayloadFormat  string
	IdempotencyKey string
	Attributes     map[string]string
}

// DeliveryAttempt 描述一次投递尝试的状态。
type DeliveryAttempt struct {
	AttemptNumber int32
	SubscriberID  string
	StartedAt     time.Time
	CompletedAt   *time.Time
	Status        string
	ErrorMessage  string
}

// RetryPlan 描述当前事件的重试策略。
type RetryPlan struct {
	MaxAttempts int32
	NextDelay   time.Duration
	Strategy    string
}

// Service 是事件投递与重试 orchestrator 的接口。
type Service interface {
	Publish(ctx context.Context, req PublishRequest) error
	Ack(ctx context.Context, deliveryID string, subscriberID string) error
	Nack(ctx context.Context, deliveryID string, subscriberID string, reason string) (RetryPlan, error)
	PollRetry(ctx context.Context, limit int) (map[string][]DeliveryAttempt, error)
}
