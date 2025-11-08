package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// TracerProvider exposes tracing helpers for plugin release flows.
type TracerProvider struct {
	tracer trace.Tracer
}

// NewTracerProvider builds an OTel tracer for a given component name.
func NewTracerProvider(component string) *TracerProvider {
	return &TracerProvider{
		tracer: otel.Tracer(component),
	}
}

// StartSpan convenience to start a span with context.
func (t *TracerProvider) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if t == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	return t.tracer.Start(ctx, name, opts...)
}
