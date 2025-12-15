package schemas

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"time"
)

// services/agent/schemas/execution.go

// ExecutionMeta 执行元数据，包含执行上下文信息
type ExecutionMeta struct {
	RequestID   string                 `json:"request_id" description:"请求ID"`
	UserID      uint64                 `json:"user_id" description:"用户ID"`
	CustomerID  uint64                 `json:"customer_id" description:"客户ID"`
	TenantUUID  string                 `json:"tenant_uuid" description:"租户UUID"`
	Timeout     time.Duration          `json:"timeout" description:"超时时间"`
	Priority    int                    `json:"priority" description:"优先级"`
	Metadata    map[string]interface{} `json:"metadata" description:"扩展元数据"`
	TraceID     string                 `json:"trace_id" description:"链路追踪ID"`
	RetryPolicy RetryPolicy            `json:"retry_policy,omitempty" description:"重试策略"`
}

// ExecutionResult 执行结果，复用 schemas.Result 并扩展执行相关信息
type ExecutionResult struct {
	Success   bool           `json:"success" description:"是否成功"`
	Data      schemas.Result `json:"data,omitempty" description:"返回数据"`
	Error     string         `json:"error,omitempty" description:"错误信息"`
	Duration  time.Duration  `json:"duration" description:"执行耗时"`
	StepID    string         `json:"step_id" description:"步骤ID"`
	Timestamp int64          `json:"timestamp" description:"时间戳"`
	Metadata  schemas.Result `json:"metadata,omitempty" description:"结果元数据"`
}

// FlowRuntimeInfo 流程运行时信息，扩展 schemas.Flow
type FlowRuntimeInfo struct {
	schemas.Flow           // 嵌入完整的流程定义
	Status       string    `json:"status" description:"运行状态"`
	Tags         []string  `json:"tags" description:"标签"`
	CreatedAt    time.Time `json:"created_at" description:"创建时间"`
	UpdatedAt    time.Time `json:"updated_at" description:"更新时间"`
}

// ExecutionStatus 执行状态，复用 schemas.PlanStatus 的概念
type ExecutionStatus struct {
	PlanID      string                 `json:"plan_id" description:"计划ID"`
	Status      string                 `json:"status" description:"状态：pending/running/completed/failed/cancelled"`
	Progress    float64                `json:"progress" description:"进度百分比 0-100"`
	CurrentStep string                 `json:"current_step" description:"当前步骤"`
	StartedAt   *time.Time             `json:"started_at,omitempty" description:"开始时间"`
	CompletedAt *time.Time             `json:"completed_at,omitempty" description:"完成时间"`
	Error       string                 `json:"error,omitempty" description:"错误信息"`
	Metadata    map[string]interface{} `json:"metadata,omitempty" description:"状态元数据"`
}
