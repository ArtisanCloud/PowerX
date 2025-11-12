package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Instruments captures Dev Hotload specific metrics.
type Instruments struct {
	RegisterLatency metric.Float64Histogram
	ReloadDuration  metric.Float64Histogram
	ActiveSessions  metric.Int64UpDownCounter
	FailureCounter  metric.Int64Counter
	attrs           []attribute.KeyValue
}

// New creates histogram/counter instruments.
func New(component string) *Instruments {
	meter := otel.Meter("github.com/ArtisanCloud/PowerX/dev_hotload")
	registerLatency, _ := meter.Float64Histogram(
		"dev.hotload.register.latency_ms",
		metric.WithUnit("ms"),
	)
	reloadDuration, _ := meter.Float64Histogram(
		"dev.hotload.reload.duration_ms",
		metric.WithUnit("ms"),
	)
	activeSessions, _ := meter.Int64UpDownCounter(
		"dev.hotload.sessions.active",
		metric.WithUnit("sessions"),
	)
	failures, _ := meter.Int64Counter(
		"dev.hotload.failures.total",
		metric.WithUnit("1"),
	)
	attrs := []attribute.KeyValue{}
	if component != "" {
		attrs = append(attrs, attribute.String("component", component))
	}
	return &Instruments{
		RegisterLatency: registerLatency,
		ReloadDuration:  reloadDuration,
		ActiveSessions:  activeSessions,
		FailureCounter:  failures,
		attrs:           attrs,
	}
}

func (i *Instruments) RecordRegisterLatency(ctx context.Context, value float64) {
	if i == nil || i.RegisterLatency == nil {
		return
	}
	i.RegisterLatency.Record(ctx, value, metric.WithAttributes(i.attrs...))
}

func (i *Instruments) RecordReloadDuration(ctx context.Context, value float64) {
	if i == nil || i.ReloadDuration == nil {
		return
	}
	i.ReloadDuration.Record(ctx, value, metric.WithAttributes(i.attrs...))
}

func (i *Instruments) IncActiveSessions(ctx context.Context, delta int64) {
	if i == nil || i.ActiveSessions == nil {
		return
	}
	i.ActiveSessions.Add(ctx, delta, metric.WithAttributes(i.attrs...))
}

func (i *Instruments) IncFailure(ctx context.Context, reason string) {
	if i == nil || i.FailureCounter == nil {
		return
	}
	attrs := i.attrs
	if reason != "" {
		attrs = append(attrs, attribute.String("reason", reason))
	}
	i.FailureCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
}
