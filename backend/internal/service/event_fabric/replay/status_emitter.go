package replay

import "context"

// StatusEmitter 用于对外推送 replay 任务状态变化（例如 WebSocket）。
type StatusEmitter interface {
	EmitReplayTaskStatus(ctx context.Context, event ReplayTaskStatusEvent)
}

// ReplayTaskStatusEvent 描述 replay 任务状态变更事件。
type ReplayTaskStatusEvent struct {
	TaskID        string `json:"task_id"`
	TenantKey     string `json:"tenant_key"`
	Topic         string `json:"topic"`
	Status        string `json:"status"`
	TraceID       string `json:"trace_id,omitempty"`
	RequestedBy   string `json:"requested_by,omitempty"`
	Shadow        bool   `json:"shadow"`
	ResultCount   int    `json:"result_count"`
	FailureReason string `json:"failure_reason,omitempty"`
	SubmittedAt   string `json:"submitted_at,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
	CancelledAt   string `json:"cancelled_at,omitempty"`
}
