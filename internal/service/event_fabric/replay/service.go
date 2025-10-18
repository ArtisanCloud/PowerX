package replay

import (
	"context"
	"time"
)

// ReplayWindow 描述回放任务的范围。
type ReplayWindow struct {
	Start time.Time
	End   time.Time
}

// ReplayTask 记录回放任务执行状态。
type ReplayTask struct {
	ID          string
	TenantID    string
	Topic       string
	TraceID     string
	Status      string
	RequestedBy string
	SubmittedAt time.Time
	CompletedAt *time.Time
}

// CreateRequest 提交新回放任务所需参数。
type CreateRequest struct {
	TenantID  string
	Topic     string
	TraceID   string
	Window    *ReplayWindow
	Reason    string
	Requester string
	Shadow    bool
}

// Service 定义事件回放生命周期。
type Service interface {
	Create(ctx context.Context, req CreateRequest) (*ReplayTask, error)
	GetProgress(ctx context.Context, taskID string) (*ReplayTask, error)
	Cancel(ctx context.Context, taskID string, operator string) error
}
