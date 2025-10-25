package domain

import (
	"context"

	"github.com/google/uuid"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const (
	contextTraceIDKey  = "trace_id"
	contextTenantIDKey = "tenant_id"
)

// Span 描述一个可结束的追踪跨度。
type Span interface {
	End(err error)
}

// Tracer 用于启动追踪跨度，实际实现可以接入 OpenTelemetry。
type Tracer interface {
	StartSpan(ctx context.Context, name string, attributes map[string]string) (context.Context, Span)
}

// Instrumentation 聚合日志与追踪依赖，方便服务层注入。
type Instrumentation struct {
	tracer Tracer
}

// NewInstrumentation 创建观测依赖，Tracer 允许通过依赖注入覆盖。
func NewInstrumentation(tracer Tracer) *Instrumentation {
	if tracer == nil {
		tracer = noopTracer{}
	}
	return &Instrumentation{tracer: tracer}
}

// Logger 返回带 context 字段的全局日志实例。
func (i *Instrumentation) Logger(ctx context.Context) *pxlog.Logger {
	return pxlog.GetGlobalLogger().WithContext(ctx)
}

// Tracer 返回可用的追踪组件。
func (i *Instrumentation) Tracer() Tracer {
	return i.tracer
}

// EnsureTraceContext 确保上下文包含 trace_id，便于日志与事件对齐。
func EnsureTraceContext(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if traceID, ok := ctx.Value(contextTraceIDKey).(string); ok && traceID != "" {
		return ctx, traceID
	}
	traceID := uuid.NewString()
	return context.WithValue(ctx, contextTraceIDKey, traceID), traceID
}

// SpanAttributes 根据上下文构建追踪属性，可附加额外标签。
func SpanAttributes(ctx context.Context, extra map[string]string) map[string]string {
	attrs := make(map[string]string, len(extra)+2)
	for k, v := range extra {
		if v != "" {
			attrs[k] = v
		}
	}
	if traceID, ok := ctx.Value(contextTraceIDKey).(string); ok && traceID != "" {
		attrs["trace.id"] = traceID
	}
	if tenantID, ok := ctx.Value(contextTenantIDKey).(string); ok && tenantID != "" {
		attrs["tenant.id"] = tenantID
	}
	return attrs
}

type noopTracer struct{}

func (noopTracer) StartSpan(ctx context.Context, _ string, _ map[string]string) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End(error) {}
