package contract

import (
	"context"

	aschemas "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

// ExternalAgentClient 统一抽象：对接 ComfyUI / SD / Coze / Dify / ...
// 注意：不新造结果类型，全部复用 services/agent/schemas 的 Execution* 实体。
type ExternalAgentClient interface {
	// 基础信息
	Name() string
	Health(ctx context.Context) error

	// Flow/Workflow 发现与元数据（平台支持则实现；不支持可返回 ErrNotSupported）
	ListFlows(ctx context.Context) ([]ExternalFlowInfo, error)
	GetFlowInfo(ctx context.Context, flowID string) (*ExternalFlowInfo, error)

	// 执行：同步/异步/流式（平台不支持的可返回 ErrNotSupported）
	Invoke(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) (*aschemas.ExecutionResult, error)
	InvokeAsync(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) (execID string, err error)
	Stream(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) (ExternalStream, error)

	// 异步任务管理
	GetStatus(ctx context.Context, execID string) (*aschemas.ExecutionStatus, error)
	GetResult(ctx context.Context, execID string) (*aschemas.ExecutionResult, error)
	Cancel(ctx context.Context, execID string) error
}

// 外部平台的流程元信息；IO Schema 直接用 flow 层 JSONSchema
type ExternalFlowInfo struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name,omitempty"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  *flowschema.JSONSchema `json:"input_schema,omitempty"`
	OutputSchema *flowschema.JSONSchema `json:"output_schema,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
}

// 流式结果：直接按 ExecutionResult 增量吐出（例如 SSE/WS 包装）
type ExternalStream interface {
	Recv() (*aschemas.ExecutionResult, error)
	Close() error
}
