package instrumentation

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// RecordCanaryPhase records latency for a deployment phase.
func (i *Instruments) RecordCanaryPhase(ctx context.Context, phase string, duration time.Duration) {
	if i == nil || i.CanaryPhaseLatency == nil {
		return
	}
	i.CanaryPhaseLatency.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("phase", phase),
	))
}

// RecordCanaryErrorRate stores the observed error rate distribution.
func (i *Instruments) RecordCanaryErrorRate(ctx context.Context, value float64) {
	if i == nil || i.CanaryErrorRate == nil {
		return
	}
	i.CanaryErrorRate.Record(ctx, value)
}

// IncrementCanaryRollback increments the auto-rollback counter.
func (i *Instruments) IncrementCanaryRollback(ctx context.Context) {
	if i == nil || i.CanaryRollbackCounter == nil {
		return
	}
	i.CanaryRollbackCounter.Add(ctx, 1)
}
