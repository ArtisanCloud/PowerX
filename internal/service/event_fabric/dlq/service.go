package dlq

import (
	"context"
	"time"
)

// Message 描述进入死信队列的事件摘要。
type Message struct {
	ID             string
	TenantID       string
	Topic          string
	EventID        string
	FailedAt       time.Time
	LastError      string
	RetryCount     int32
	ReplayEligible bool
}

// ListRequest 用于分页查询死信消息。
type ListRequest struct {
	TenantID string
	Topic    string
	Status   string
	Page     int
	PageSize int
}

// ReplayRequest 描述批量重放参数。
type ReplayRequest struct {
	MessageIDs []string
	OperatorID string
	Notes      string
}

// Service 定义死信管理能力。
type Service interface {
	List(ctx context.Context, req ListRequest) ([]*Message, int64, error)
	Replay(ctx context.Context, req ReplayRequest) (int, error)
	Purge(ctx context.Context, tenantID string, topic string) (int, error)
}
