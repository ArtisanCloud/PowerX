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

	// Common metric attribute keys. 观测标签需至少包含能力、插件与协议，方便 Trace/事件串联。
	LabelCapabilityID = "capability_id"
	LabelPluginID     = "plugin_id"
	LabelProtocol     = "protocol"
	LabelTenantUUID   = "tenant_uuid"
	LabelTraceID      = "trace_id"
	LabelResult       = "result"
	LabelFallback     = "fallback"
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
