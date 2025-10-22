package instrumentation

import (
	"context"

	"github.com/google/uuid"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const (
	contextTraceIDKey  = "trace_id"
	contextTenantIDKey = "tenant_id"
)

// Span 定义可结束的追踪跨度。
type Span interface {
	End(err error)
}

// Tracer 描述可被注入的追踪实现。
type Tracer interface {
	StartSpan(ctx context.Context, name string, attributes map[string]string) (context.Context, Span)
}

// Instrumentation 聚合日志与追踪能力。
type Instrumentation struct {
	tracer Tracer
}

// NewInstrumentation 创建观测依赖。
func NewInstrumentation(tracer Tracer) *Instrumentation {
	if tracer == nil {
		tracer = noopTracer{}
	}
	return &Instrumentation{tracer: tracer}
}

// Logger 返回带上下文字段的全局日志实例。
func (i *Instrumentation) Logger(ctx context.Context) *pxlog.Logger {
	return pxlog.GetGlobalLogger().WithContext(ctx)
}

// Tracer 返回配置的追踪实例。
func (i *Instrumentation) Tracer() Tracer {
	return i.tracer
}

// EnsureTraceContext 确保上下文包含 trace_id，方便日志与事件对齐。
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

// WithTenant 将 tenant_id 写入上下文。
func WithTenant(ctx context.Context, tenantID string) context.Context {
	if ctx == nil {
		return context.WithValue(context.Background(), contextTenantIDKey, tenantID)
	}
	return context.WithValue(ctx, contextTenantIDKey, tenantID)
}

// SpanAttributes 合成标准属性，额外标签可通过 extra 传入。
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
