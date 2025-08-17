package contract

import (
	"context"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	aschemas "github.com/ArtisanCloud/PowerX/services/agent/schemas"
	"github.com/cloudwego/eino/schema"
)

// Agent 智能体接口，提供通用的智能体能力
type Agent interface {

	// GetInfo 获取智能体信息
	GetInfo() *aschemas.AgentInfo

	// ListFlows 获取可用的流程列表（简化信息）
	ListFlows(ctx context.Context, meta aschemas.ExecutionMeta) ([]aschemas.FlowRuntimeInfo, error)

	// GetFlowInfo 获取指定流程的详细信息（完整定义）
	GetFlowInfo(ctx context.Context, flowID string, meta aschemas.ExecutionMeta) (*aschemas.FlowRuntimeInfo, error)

	// ValidateParams 验证流程参数，使用 flowschema.JSONSchema 进行验证
	ValidateParams(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) error

	// Invoke 同步执行流程
	Invoke(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) (*aschemas.ExecutionResult, error)

	// InvokeAsync 异步执行流程，返回执行ID
	InvokeAsync(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) (string, error)

	// Stream 流式执行流程
	Stream(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) (*schema.StreamReader[*aschemas.ExecutionResult], error)

	// GetExecutionStatus 获取异步执行状态
	GetExecutionStatus(ctx context.Context, executionID string, meta aschemas.ExecutionMeta) (*aschemas.ExecutionStatus, error)

	// CancelExecution 取消执行
	CancelExecution(ctx context.Context, executionID string, meta aschemas.ExecutionMeta) error

	// GetExecutionResult 获取执行结果
	GetExecutionResult(ctx context.Context, executionID string, meta aschemas.ExecutionMeta) (*aschemas.ExecutionResult, error)

	// GetMetrics 获取智能体指标
	GetMetrics(ctx context.Context, meta aschemas.ExecutionMeta) (flowschema.Result, error)

	// Health 健康检查
	Health(ctx context.Context) error

	// Shutdown 优雅关闭
	Shutdown(ctx context.Context) error
}
