package schemas

import "time"

// 运行时状态（可在 StatusHandler 里返回）
type Runtime struct {
	CurrentFlow   string    `json:"current_flow,omitempty"`    // 当前执行的 Flow ID
	CurrentStepID string    `json:"current_step_id,omitempty"` // 当前执行的步骤 ID
	QueueLength   int       `json:"queue_length"`              // 等待队列长度
	ActiveJobs    int       `json:"active_jobs"`               // 当前活跃任务数量
	LastError     string    `json:"last_error,omitempty"`      // 最近一次错误
	Version       string    `json:"version,omitempty"`         // Agent 运行版本
	UpdatedAt     time.Time `json:"updated_at"`                // 状态更新时间
}
