package metrics

import (
	"context"
	"errors"
	"time"
)

const (
	// MetricCapabilityInvokeTotal counts every Capability invocation routed via Registry/Selector.
	MetricCapabilityInvokeTotal = "powerx_capability_invoke_total"
	// MetricCapabilityInvokeLatencyMS records invocation latency in milliseconds.
	MetricCapabilityInvokeLatencyMS = "powerx_capability_invoke_latency_ms"
	// MetricCapabilityInvokeErrorTotal counts failed invocations.
	MetricCapabilityInvokeErrorTotal = "powerx_capability_invoke_error_total"

	// Workflow-specific metrics.
	MetricWorkflowTemplateSnapshotTotal = "powerx_workflow_template_snapshot_total"
	MetricWorkflowInvocationTotal       = "powerx_workflow_invocation_total"
	MetricWorkflowInvocationErrorTotal  = "powerx_workflow_invocation_error_total"

	// Common metric attribute keys. 观测标签需至少包含能力、插件与协议，方便 Trace/事件串联。
	LabelCapabilityID = "capability_id"
	LabelPluginID     = "plugin_id"
	LabelProtocol     = "protocol"
	LabelTenantUUID   = "tenant_uuid"
	LabelTraceID      = "trace_id"
	LabelResult       = "result"
	LabelFallback     = "fallback"
	LabelTemplateID   = "template_id"
	LabelNeedsUpgrade = "needs_upgrade"
)

const (
	// ResultSuccess 表示调用成功。
	ResultSuccess = "success"
	// ResultFallback 表示触发降级后成功。
	ResultFallback = "fallback"
	// ResultFailed 表示最终失败。
	ResultFailed = "failed"
)

// MetricEmitter 抽象底层指标上报实现，可由 Prometheus/OpenTelemetry/自定义埋点实现。
type MetricEmitter interface {
	Counter(ctx context.Context, name string, value float64, attrs map[string]string)
	Histogram(ctx context.Context, name string, value float64, attrs map[string]string)
}

// CapabilityInvocationLabels 描述一次指标上报所需的上下文。
type CapabilityInvocationLabels struct {
	CapabilityID string
	PluginID     string
	Protocol     string
	TenantUUID   string
	TraceID      string
	Result       string
	Fallback     bool
}

// CapabilityInvocationSample 描述记录时的输入参数。
type CapabilityInvocationSample struct {
	Labels  CapabilityInvocationLabels
	Latency time.Duration
	Err     error
}

// WorkflowCatalogSample 描述 Workflow Catalog 快照中的单个模板指标。
type WorkflowCatalogSample struct {
	TemplateID   string
	CapabilityID string
	PluginID     string
	NeedsUpgrade bool
}

// WorkflowExecutionSample 描述 Workflow Engine 对模板执行的指标。
type WorkflowExecutionSample struct {
	TemplateID   string
	CapabilityID string
	PluginID     string
	TenantUUID   string
	Protocol     string
	Status       string
	NeedsUpgrade bool
	Err          error
}

// CapabilityRegistryMetrics 提供统一的能力调用指标上报。
type CapabilityRegistryMetrics struct {
	emitter MetricEmitter
}

// NewCapabilityRegistryMetrics 构造 Recorder，未注入 emitter 时使用 no-op。
func NewCapabilityRegistryMetrics(emitter MetricEmitter) *CapabilityRegistryMetrics {
	if emitter == nil {
		emitter = noopEmitter{}
	}
	return &CapabilityRegistryMetrics{emitter: emitter}
}

// ObserveInvocation 记录一次调用的总数、延迟与错误计数。
func (m *CapabilityRegistryMetrics) ObserveInvocation(ctx context.Context, sample CapabilityInvocationSample) {
	if m == nil {
		return
	}
	attrs := map[string]string{
		LabelCapabilityID: sample.Labels.CapabilityID,
		LabelPluginID:     sample.Labels.PluginID,
		LabelProtocol:     sample.Labels.Protocol,
		LabelResult:       normalizeResult(sample),
	}
	if sample.Labels.TenantUUID != "" {
		attrs[LabelTenantUUID] = sample.Labels.TenantUUID
	}
	if sample.Labels.TraceID != "" {
		attrs[LabelTraceID] = sample.Labels.TraceID
	}
	if sample.Labels.Fallback {
		attrs[LabelFallback] = "true"
	}

	m.emitter.Counter(ctx, MetricCapabilityInvokeTotal, 1, attrs)

	if sample.Latency > 0 {
		m.emitter.Histogram(ctx, MetricCapabilityInvokeLatencyMS, float64(sample.Latency.Milliseconds()), attrs)
	}

	if sample.Err != nil {
		errAttrs := cloneMap(attrs)
		var target interface{ Error() string }
		if errors.As(sample.Err, &target) {
			errAttrs["error"] = target.Error()
		}
		m.emitter.Counter(ctx, MetricCapabilityInvokeErrorTotal, 1, errAttrs)
	}
}

// ObserveWorkflowCatalog 记录 Workflow Catalog 快照中的模板 adoption 状态。
func (m *CapabilityRegistryMetrics) ObserveWorkflowCatalog(ctx context.Context, samples []WorkflowCatalogSample) {
	if m == nil || len(samples) == 0 {
		return
	}
	for _, sample := range samples {
		attrs := map[string]string{
			LabelCapabilityID: sample.CapabilityID,
			LabelPluginID:     sample.PluginID,
		}
		if sample.TemplateID != "" {
			attrs[LabelTemplateID] = sample.TemplateID
		}
		if sample.NeedsUpgrade {
			attrs[LabelNeedsUpgrade] = "true"
		} else {
			attrs[LabelNeedsUpgrade] = "false"
		}
		m.emitter.Counter(ctx, MetricWorkflowTemplateSnapshotTotal, 1, attrs)
	}
}

// ObserveWorkflowExecution 记录 Workflow Engine 对模板执行时的成功率。
func (m *CapabilityRegistryMetrics) ObserveWorkflowExecution(ctx context.Context, sample WorkflowExecutionSample) {
	if m == nil {
		return
	}
	attrs := map[string]string{
		LabelCapabilityID: sample.CapabilityID,
		LabelProtocol:     sample.Protocol,
		LabelResult:       normalizeWorkflowExecutionResult(sample),
	}
	if sample.TemplateID != "" {
		attrs[LabelTemplateID] = sample.TemplateID
	}
	if sample.PluginID != "" {
		attrs[LabelPluginID] = sample.PluginID
	}
	if sample.TenantUUID != "" {
		attrs[LabelTenantUUID] = sample.TenantUUID
	}
	if sample.NeedsUpgrade {
		attrs[LabelNeedsUpgrade] = "true"
	}
	m.emitter.Counter(ctx, MetricWorkflowInvocationTotal, 1, attrs)
	if sample.Err != nil {
		errAttrs := cloneMap(attrs)
		errAttrs["error"] = sample.Err.Error()
		m.emitter.Counter(ctx, MetricWorkflowInvocationErrorTotal, 1, errAttrs)
	}
}

func normalizeResult(sample CapabilityInvocationSample) string {
	if sample.Labels.Result != "" {
		return sample.Labels.Result
	}
	if sample.Err != nil {
		return ResultFailed
	}
	if sample.Labels.Fallback {
		return ResultFallback
	}
	return ResultSuccess
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type noopEmitter struct{}

func (noopEmitter) Counter(context.Context, string, float64, map[string]string)   {}
func (noopEmitter) Histogram(context.Context, string, float64, map[string]string) {}

func normalizeWorkflowExecutionResult(sample WorkflowExecutionSample) string {
	if sample.Status != "" {
		return sample.Status
	}
	if sample.Err != nil {
		return ResultFailed
	}
	return ResultSuccess
}
