package instrumentation

import (
	"context"

	"github.com/google/uuid"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

type contextKey string

const (
	traceIDKey  contextKey = "agent_model_hub_trace_id"
	tenantIDKey contextKey = "agent_model_hub_tenant_id"
)

// Span represents an active tracing span.
type Span interface {
	End(err error)
}

// Tracer abstracts OpenTelemetry-compatible tracer implementations.
type Tracer interface {
	StartSpan(ctx context.Context, name string, attrs map[string]string) (context.Context, Span)
}

// Meter exposes a simple metric recorder abstraction.
type Meter interface {
	Record(ctx context.Context, name string, value float64, attrs map[string]string)
}

// Instrumentation aggregates logging + tracing + metrics helpers.
type Instrumentation struct {
	tracer Tracer
	meter  Meter
}

// NewInstrumentation builds a helper with optional tracer/meter (defaults to no-op).
func NewInstrumentation(tracer Tracer, meter Meter) *Instrumentation {
	if tracer == nil {
		tracer = noopTracer{}
	}
	if meter == nil {
		meter = noopMeter{}
	}
	return &Instrumentation{tracer: tracer, meter: meter}
}

// Logger returns a context-aware logger.
func (i *Instrumentation) Logger(ctx context.Context) *pxlog.Logger {
	return pxlog.GetGlobalLogger().WithContext(ctx)
}

// Tracer exposes the configured tracer implementation.
func (i *Instrumentation) Tracer() Tracer {
	return i.tracer
}

// RecordMetric emits a measurement with trace/tenant attributes.
func (i *Instrumentation) RecordMetric(ctx context.Context, name string, value float64, attrs map[string]string) {
	if attrs == nil {
		attrs = map[string]string{}
	}
	for k, v := range contextAttributes(ctx) {
		if _, exists := attrs[k]; !exists {
			attrs[k] = v
		}
	}
	i.meter.Record(ctx, name, value, attrs)
}

// EnsureTraceContext guarantees trace context on the provided ctx.
func EnsureTraceContext(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if id, ok := ctx.Value(traceIDKey).(string); ok && id != "" {
		return ctx, id
	}
	traceID := uuid.NewString()
	return context.WithValue(ctx, traceIDKey, traceID), traceID
}

// WithTenant annotates context with tenant identifier for telemetry attributes.
func WithTenant(ctx context.Context, tenant string) context.Context {
	if tenant == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantIDKey, tenant)
}

// SpanAttributes merges trace metadata with custom labels.
func SpanAttributes(ctx context.Context, extra map[string]string) map[string]string {
	attrs := make(map[string]string, len(extra)+2)
	for k, v := range extra {
		if v != "" {
			attrs[k] = v
		}
	}
	for k, v := range contextAttributes(ctx) {
		if _, exists := attrs[k]; !exists {
			attrs[k] = v
		}
	}
	return attrs
}

func contextAttributes(ctx context.Context) map[string]string {
	attrs := map[string]string{}
	if id, ok := ctx.Value(traceIDKey).(string); ok && id != "" {
		attrs["trace.id"] = id
	}
	if tenant, ok := ctx.Value(tenantIDKey).(string); ok && tenant != "" {
		attrs["tenant.id"] = tenant
	}
	return attrs
}

type noopTracer struct{}

func (noopTracer) StartSpan(ctx context.Context, _ string, _ map[string]string) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End(error) {}

type noopMeter struct{}

func (noopMeter) Record(context.Context, string, float64, map[string]string) {}
