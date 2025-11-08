package adapters

import (
	"context"

	acontract "github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	aschemas "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/cloudwego/eino/schema"
)

// Bridge: 把 ExternalAgentClient 包成“Agent”语义，供内核统一使用。
type Bridge struct {
	info *aschemas.AgentInfo
	cli  acontract.ExternalAgentClient
}

func NewBridge(info *aschemas.AgentInfo, cli acontract.ExternalAgentClient) *Bridge {
	return &Bridge{info: info, cli: cli}
}

func (b *Bridge) GetInfo() *aschemas.AgentInfo { return b.info }

func (b *Bridge) ListFlows(ctx context.Context, meta aschemas.ExecutionMeta) ([]aschemas.FlowRuntimeInfo, error) {
	ls, err := b.cli.ListFlows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]aschemas.FlowRuntimeInfo, 0, len(ls))
	for _, it := range ls {
		out = append(out, aschemas.FlowRuntimeInfo{
			// 这里只返回最小必要信息；需要完整 Flow 可在 GetFlowInfo 中填充
			Status: "ready",
			Tags:   it.Tags,
		})
	}
	return out, nil
}

func (b *Bridge) GetFlowInfo(ctx context.Context, flowID string, meta aschemas.ExecutionMeta) (*aschemas.FlowRuntimeInfo, error) {
	fi, err := b.cli.GetFlowInfo(ctx, flowID)
	if err != nil {
		return nil, err
	}
	// 如平台无法返回完整编排，可只返回元信息；与内核的 Flow 定义解耦
	return &aschemas.FlowRuntimeInfo{
		Status: "ready",
		Tags:   fi.Tags,
	}, nil
}

func (b *Bridge) ValidateParams(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) error {
	// 若平台提供 InputSchema，可在此校验；否则直过
	return nil
}

func (b *Bridge) Invoke(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) (*aschemas.ExecutionResult, error) {
	return b.cli.Invoke(ctx, flowID, params, meta)
}

func (b *Bridge) InvokeAsync(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) (string, error) {
	return b.cli.InvokeAsync(ctx, flowID, params, meta)
}

func (b *Bridge) Stream(ctx context.Context, flowID string, params flowschema.Context, meta aschemas.ExecutionMeta) (*schema.StreamReader[*aschemas.ExecutionResult], error) {
	ext, err := b.cli.Stream(ctx, flowID, params, meta)
	if err != nil {
		return nil, err
	}
	return toEinoStream(ext), nil
}

func (b *Bridge) GetExecutionStatus(ctx context.Context, executionID string, meta aschemas.ExecutionMeta) (*aschemas.ExecutionStatus, error) {
	return b.cli.GetStatus(ctx, executionID)
}

func (b *Bridge) CancelExecution(ctx context.Context, executionID string, meta aschemas.ExecutionMeta) error {
	return b.cli.Cancel(ctx, executionID)
}

func (b *Bridge) GetExecutionResult(ctx context.Context, executionID string, meta aschemas.ExecutionMeta) (*aschemas.ExecutionResult, error) {
	return b.cli.GetResult(ctx, executionID)
}

func (b *Bridge) GetMetrics(ctx context.Context, meta aschemas.ExecutionMeta) (flowschema.Result, error) {
	// 外部平台通常无统一 metrics；这里返回空 map
	return flowschema.Result{}, nil
}

func (b *Bridge) Health(ctx context.Context) error   { return b.cli.Health(ctx) }
func (b *Bridge) Shutdown(ctx context.Context) error { return nil }
